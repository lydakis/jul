package gitutil

import "testing"

func TestExtractChangeID(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "gerrit trailer",
			message: "feat: add thing\n\nChange-Id: I0123456789abcdef0123456789abcdef01234567\n",
			want:    "I0123456789abcdef0123456789abcdef01234567",
		},
		{
			name:    "lowercase",
			message: "fix: test\n\nchange-id: Iaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n",
			want:    "Iaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		{
			name:    "missing",
			message: "chore: update",
			want:    "",
		},
	}

	for _, tc := range cases {
		if got := ExtractChangeID(tc.message); got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}
