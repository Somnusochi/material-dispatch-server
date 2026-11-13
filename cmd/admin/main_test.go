package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAdminRequiresExplicitCredentials(t *testing.T) {
	for _, args := range [][]string{{}, {"--username", "admin"}, {"--username", "admin", "--name", "管理员", "--password-stdin"}} {
		var out bytes.Buffer
		if err := run(args, strings.NewReader(""), &out); err == nil {
			t.Fatalf("accepted missing credentials: %v", args)
		}
		if strings.Contains(out.String(), "password") {
			t.Fatal("credential output")
		}
	}
}
