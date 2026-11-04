package porttest

import "testing"

func TestReadReturnsAnIndependentCopy(t *testing.T) {
	data := []byte("notes")
	files := Files{Data: map[string][]byte{"input": data}}
	got, failure := files.Read("input")
	if failure != nil {
		t.Fatal(failure)
	}
	got[0] = 'N'
	if string(files.Data["input"]) != "notes" || string(data) != "notes" {
		t.Errorf("Read() exposed mutable fake state: map=%q source=%q", files.Data["input"], data)
	}
}
