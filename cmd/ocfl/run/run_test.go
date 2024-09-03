package run_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/cmd/ocfl/run"
)

var (
	contentFixture = filepath.Join(`..`, `..`, `..`, `testdata`, `content-fixture`)
	allLayouts     = []string{
		"0002-flat-direct-storage-layout",
		"0003-hash-and-id-n-tuple-storage-layout",
		"0004-hashed-n-tuple-storage-layout",
		// "0006-flat-omit-prefix-storage-layout",
		"0007-n-tuple-omit-prefix-storage-layout",
	}
)

func testRun(args []string, expect func(err error, stdout, stderr string)) {
	ctx := context.Background()
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	args = append([]string{"ocfl"}, args...)
	err := run.CLI(ctx, args, stdout, stderr)
	expect(err, stdout.String(), stderr.String())
}

func TestAllLayouts(t *testing.T) {
	for _, l := range allLayouts {
		t.Run(l, func(t *testing.T) {
			tmpDir := t.TempDir()
			rootDesc := "test description"
			args := []string{
				"init-root",
				"--description", rootDesc,
				"--root", tmpDir,
				"--layout", l,
			}
			testRun(args, func(err error, stdout string, stderr string) {
				be.NilErr(t, err)
				be.True(t, strings.Contains(stdout, tmpDir))
				be.True(t, strings.Contains(stdout, l))
				be.True(t, strings.Contains(stdout, rootDesc))
			})
			// ocfl commit
			objID := "object-01"
			args = []string{
				"commit",
				contentFixture,
				"--root", tmpDir,
				"--id", objID,
				"--message", "my message",
				"--name", "Me",
				"--email", "me@domain.net",
			}
			testRun(args, func(err error, _ string, _ string) {
				be.NilErr(t, err)
			})
			// ocfl ls
			args = []string{
				"ls",
				"--root", tmpDir,
				"--id", objID,
			}
			testRun(args, func(err error, stdout string, _ string) {
				be.NilErr(t, err)
				be.True(t, strings.Contains(stdout, "hello.csv"))
				be.True(t, strings.Contains(stdout, "folder1/file.txt"))
			})

		})
	}
}
