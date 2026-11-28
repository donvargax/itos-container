package features

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCredentialRunPolicyEditBracket(t *testing.T) {
	for _, tc := range []struct {
		name, op     string
		write, after bool
	}{
		{name: "replacement_alone", op: "rename"},
		{name: "unlink_alone", op: "unlink"},
		{name: "write_before_replacement", op: "rename", write: true},
		{name: "write_after_replacement", op: "rename", write: true, after: true},
		{name: "write_before_unlink", op: "unlink", write: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, policy := credentialBracketFixture(t)
			var injected bool
			err := c.editPolicy(tc.op, []byte("replacement"), func(after bool) error {
				if !tc.write || after != tc.after {
					return nil
				}
				f, err := os.OpenFile(policy, os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, first := f.WriteString("first")
				_, second := f.WriteString("second")
				if err := errors.Join(first, second, f.Close()); err != nil {
					t.Fatal(err)
				}
				injected = true
				return nil
			})
			if tc.write {
				if !injected || err == nil || !strings.Contains(err.Error(), "policy bracket events differ") {
					t.Fatalf("observer did not reject the two writes: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("unpaired_control", func(t *testing.T) {
		for _, begin := range []bool{true, false} {
			c, policy := credentialBracketFixture(t)
			bracket := c.bracket(1, policy, "rename", []uint32{credentialMoveTo})
			c.brackets = append(c.brackets, bracket)
			path := bracket.end
			if begin {
				path = bracket.begin
			}
			if err := credentialRunSentinel(path); err != nil {
				t.Fatal(err)
			}
			if c.judge() == nil {
				t.Fatal("accepted an unmatched policy sentinel")
			}
		}
	})
	t.Run("new_directory", func(t *testing.T) {
		c, _ := credentialBracketFixture(t)
		path := filepath.Join(c.checkout, "new-directory", "new-file")
		if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("unexpected"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := c.judge(); err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("new directory contents escaped attribution: %v", err)
		}
	})
	t.Run("overflow", func(t *testing.T) {
		if judgeCredentialRunEvents([]credentialRunEvent{{kind: credentialOverflow}}, "", "", nil, nil, nil) == nil {
			t.Fatal("accepted observer overflow")
		}
	})
	t.Run("journal_mismatch", func(t *testing.T) {
		c, policy := credentialBracketFixture(t)
		if err := c.editPolicy("rename", []byte("replacement"), nil); err != nil {
			t.Fatal(err)
		}
		rename := c.mutations[0]
		for _, journal := range [][]credentialRunMutation{{rename, rename}, {{Op: "open-write", Path: policy, At: time.Now()}}} {
			c.mutations = journal
			if err := c.judge(); err == nil || !strings.Contains(err.Error(), "journal") {
				t.Fatalf("accepted inaccurate fixture journal: %v", err)
			}
		}
	})
}

func credentialBracketFixture(t *testing.T) (*credentialRunCheckout, string) {
	t.Helper()
	root := t.TempDir()
	checkout, control, staging := filepath.Join(root, "checkout"), filepath.Join(root, "control"), filepath.Join(root, "stage")
	for _, path := range []string{checkout, control, staging} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	policy := filepath.Join(checkout, "itos-container.yaml")
	if err := os.WriteFile(policy, []byte("original policy"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := armCredentialRunCheckout(checkout, control, staging)
	if errors.Is(err, errCredentialRunObserverUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.close(); err != nil {
			t.Error(err)
		}
	})
	if err := c.prove(); err != nil {
		t.Fatal(err)
	}
	return c, policy
}
