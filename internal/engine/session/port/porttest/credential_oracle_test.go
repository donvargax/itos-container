package porttest

import "testing"

type credentialReporter struct{ failures int }

func (*credentialReporter) Helper()                 {}
func (r *credentialReporter) Errorf(string, ...any) { r.failures++ }
func TestCredentialOracleRejectsTrimmedNewline(t *testing.T) {
	report := &credentialReporter{}
	CredentialStagingContract(report, func(values map[string][]byte) CredentialFixture {
		f := MemoryCredentialFixture(values)
		f.Stager.(*CredentialStager).Fault = "trim newline"
		return f
	})
	if report.failures < 6 {
		t.Fatal("shared copy oracle accepted a fake trimming final newlines")
	}
}
