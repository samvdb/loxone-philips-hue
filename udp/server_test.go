package udp

import (
	"strings"
	"testing"
)

func TestParseCommand_Valid(t *testing.T) {
	tests := []struct {
		name string
		line string
		want Command
	}{
		{
			name: "light on true",
			line: "/grouped_light/abc-123/on true",
			want: Command{
				Domain: "light",
				ID:     "abc-123",
				Action: "on",
				Value:  "true",
			},
		},
		{
			name: "light on 1",
			line: "/grouped_light/abc-123/on 1",
			want: Command{
				Domain: "light",
				ID:     "abc-123",
				Action: "on",
				Value:  "1",
			},
		},
		{
			name: "light on 0",
			line: "/grouped_light/abc-123/on 0",
			want: Command{
				Domain: "light",
				ID:     "abc-123",
				Action: "on",
				Value:  "0",
			},
		},
		{
			name: "light dimmable mid value",
			line: "/grouped_light/abc-123/dimmable 50",
			want: Command{
				Domain: "light",
				ID:     "abc-123",
				Action: "dimmable",
				Value:  "50",
			},
		},
		{
			name: "light dimmable 0",
			line: "/grouped_light/abc-123/dimmable 0",
			want: Command{
				Domain: "light",
				ID:     "abc-123",
				Action: "dimmable",
				Value:  "0",
			},
		},
		{
			name: "light dimmable 100",
			line: "/grouped_light/abc-123/dimmable 100",
			want: Command{
				Domain: "light",
				ID:     "abc-123",
				Action: "dimmable",
				Value:  "100",
			},
		},
		{
			name: "extra whitespace",
			line: "   /grouped_light/abc-123/on   true   ",
			want: Command{
				Domain: "light",
				ID:     "abc-123",
				Action: "on",
				Value:  "true",
			},
		},
	}

	for _, tt := range tests {
		tt := tt // capture range var
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseCommand(tt.line)
			if err != nil {
				t.Fatalf("parseCommand() unexpected error: %v", err)
			}

			if got.Domain != tt.want.Domain {
				t.Errorf("Domain = %q, want %q", got.Domain, tt.want.Domain)
			}
			if got.ID != tt.want.ID {
				t.Errorf("ID = %q, want %q", got.ID, tt.want.ID)
			}
			if got.Action != tt.want.Action {
				t.Errorf("Action = %q, want %q", got.Action, tt.want.Action)
			}
			if got.Value != tt.want.Value {
				t.Errorf("Value = %q, want %q", got.Value, tt.want.Value)
			}
		})
	}
}

func TestParseCommand_Invalid(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantErrSubstr string
	}{
		{
			name:          "empty line",
			line:          "",
			wantErrSubstr: "expected '<path> <value>'",
		},
		{
			name:          "missing value",
			line:          "/grouped_light/abc-123/on",
			wantErrSubstr: "expected '<path> <value>'",
		},
		{
			name:          "bad path no leading slash",
			line:          "light/abc-123/on true",
			wantErrSubstr: "bad path",
		},
		{
			name:          "too few segments",
			line:          "/grouped_light/on true",
			wantErrSubstr: "bad path",
		},
		{
			name:          "unsupported domain",
			line:          "/sensor/abc-123/on true",
			wantErrSubstr: "unsupported domain",
		},
		{
			name:          "unsupported action",
			line:          "/grouped_light/abc-123/blink true",
			wantErrSubstr: "unsupported action",
		},
		{
			name:          "on invalid value string",
			line:          "/grouped_light/abc-123/on maybe",
			wantErrSubstr: "on expects true|false|1|0",
		},
		{
			name:          "dimmable non-numeric",
			line:          "/grouped_light/abc-123/dimmable high",
			wantErrSubstr: "dimmable expects 0..100",
		},
		{
			name:          "dimmable negative",
			line:          "/grouped_light/abc-123/dimmable -1",
			wantErrSubstr: "dimmable expects 0..100",
		},
		{
			name:          "dimmable above 100",
			line:          "/grouped_light/abc-123/dimmable 101",
			wantErrSubstr: "dimmable expects 0..100",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range var
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseCommand(tt.line)
			if err == nil {
				t.Fatalf("parseCommand() expected error, got nil")
			}
			if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Fatalf("parseCommand() error = %q, want to contain %q", err.Error(), tt.wantErrSubstr)
			}
		})
	}
}
