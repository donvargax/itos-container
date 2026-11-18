package porttest

import "strings"

// ReadyRecordCase is one complete stdout stream delivered to readiness, or one
// candidate line delivered to the copy filter. Accepted is true exactly when
// startup accepts it and a stored log omits it.
type ReadyRecordCase struct {
	Name, Line string
	Accepted   bool
}

func ReadyRecords() []ReadyRecordCase {
	ready := strings.SplitAfter(ProxyStdout, "\n")[0]
	body := strings.TrimSuffix(ready, "\n")
	return []ReadyRecordCase{
		{"ready", ready, true},
		{"reordered keys", `{"event":"ready","time":"2026-11-09T12:00:00Z","seq":1,"schema":1}` + "\n", true},
		{"fractional seconds", strings.Replace(ready, "12:00:00Z", "12:00:00.123456789Z", 1), true},
		{"spaces inside", `{ "schema" : 1 , "seq" : 1 , "time" : "2026-11-09T12:00:00Z" , "event" : "ready" }` + "\n", true},
		{"leading spaces to 2,048 bytes", strings.Repeat(" ", 2048-len(body)) + ready, true},
		{"leading spaces to 2,049 bytes", strings.Repeat(" ", 2049-len(body)) + ready, false},
		{"stderr looking stdout", "failure\n", false},
		{"extra key", `{"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready","extra":0}` + "\n", false},
		{"wrong seq", strings.Replace(ready, `"seq":1`, `"seq":2`, 1), false},
		{"wrong event", strings.Replace(ready, "ready", "refused", 1), false},
		{"wrong time", strings.Replace(ready, "Z", "+00:00", 1), false},
		{"long", strings.Repeat(" ", 2049) + "\n", false},
		{"trailing space", body + " \n", false},
		{"trailing carriage return", body + "\r\n", false},
		{"trailing object", body + "{}\n", false},
		{"repeated key", `{"schema":1,"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready"}` + "\n", false},
		{"repeated key in place of one", `{"schema":1,"schema":1,"time":"2026-11-09T12:00:00Z","event":"ready"}` + "\n", false},
		{"schema 1.0", strings.Replace(ready, `"schema":1`, `"schema":1.0`, 1), false},
		{"schema 2", strings.Replace(ready, `"schema":1`, `"schema":2`, 1), false},
		{"schema as text", strings.Replace(ready, `"schema":1`, `"schema":"1"`, 1), false},
		{"seq as text", strings.Replace(ready, `"seq":1`, `"seq":"1"`, 1), false},
		{"event as number", strings.Replace(ready, `"ready"`, `1`, 1), false},
		{"time as number", strings.Replace(ready, `"2026-11-09T12:00:00Z"`, `1`, 1), false},
		{"time that is no date", strings.Replace(ready, "2026-11-09T12:00:00Z", "soon, Z", 1), false},
		{"no event", `{"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z"}` + "\n", false},
		{"another key in place of schema", `{"other":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready"}` + "\n", false},
		{"array", `[1,1,"2026-11-09T12:00:00Z","ready"]` + "\n", false},
		{"unclosed object", strings.TrimSuffix(body, "}") + "\n", false},
		{"missing value", `{"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":}` + "\n", false},
		{"key without value", `{"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event"}` + "\n", false},
		{"unquoted key", `{schema:1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready"}` + "\n", false},
		{"empty line", "\n", false},
		{"empty object", "{}\n", false},

		{"EOF inside object", "{", false},
		{"EOF after ready object", body, false},
	}
}
