// Tests for the Phase 0 CLI skeleton: the version and help verbs, the
// no-command case, and unknown commands. These lock the exit-code
// contract (0 success, 2 usage) before the capability verbs are added.
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string // substring, empty means "expect empty"
		wantStderr string // substring, empty means "expect empty"
	}{
		{
			name:       "version",
			args:       []string{"version"},
			wantCode:   0,
			wantStdout: "osirisjson-consumer dev (commit unknown)",
		},
		{
			name:       "help",
			args:       []string{"help"},
			wantCode:   0,
			wantStdout: "Usage:",
		},
		{
			name:       "help short flag",
			args:       []string{"-h"},
			wantCode:   0,
			wantStdout: "Usage:",
		},
		{
			name:       "no command",
			args:       nil,
			wantCode:   2,
			wantStderr: "no command given",
		},
		{
			name:       "unknown command",
			args:       []string{"report"},
			wantCode:   2,
			wantStderr: `unknown command "report"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if tt.wantStdout == "" {
				if stdout.Len() != 0 {
					t.Errorf("stdout = %q, want empty", stdout.String())
				}
			} else if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want substring %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
