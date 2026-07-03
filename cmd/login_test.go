package cmd

import "testing"

func TestNormalizeToken(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"clean", "sk-ant-oat01-abc", "sk-ant-oat01-abc"},
		{"trailing newline", "sk-ant-oat01-abc\n", "sk-ant-oat01-abc"},
		{"wrapped paste LF", "sk-ant-oat01-aaaa\nbbbb\ncccc", "sk-ant-oat01-aaaabbbbcccc"},
		{"wrapped paste CR", "sk-ant-oat01-aaaa\rbbbb\rcccc\r", "sk-ant-oat01-aaaabbbbcccc"},
		{"CRLF and spaces", " sk-ant-oat01-aa\r\nbb \tcc ", "sk-ant-oat01-aabbcc"},
		{"control bytes", "sk-ant-oat01-ab\x04\x1bcd", "sk-ant-oat01-abcd"},
		{"wrap-point padding spaces", "sk-ant-oat01-aa   \r   bb  \r", "sk-ant-oat01-aabb"},
		{"bracketed paste markers", "\x1b[200~sk-ant-oat01-br\x1b[201~", "sk-ant-oat01-br"},
	}
	for _, c := range cases {
		if got := normalizeToken(c.in); got != c.want {
			t.Errorf("%s: normalizeToken(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
