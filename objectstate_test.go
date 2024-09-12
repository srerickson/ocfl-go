package ocfl_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
)

func TestParseObjectRootEntries(t *testing.T) {
	const objDecl = "0=ocfl_object_1.1"
	type testCase struct {
		input []fs.DirEntry
		want  ocfl.ObjectState
	}
	testCases := map[string]testCase{
		"nil input": {
			input: nil,
			want:  ocfl.ObjectState{},
		},
		"empty input": {
			input: []fs.DirEntry{},
			want:  ocfl.ObjectState{},
		},
		"regular namaste": {
			input: []fs.DirEntry{
				&dirEntry{name: objDecl},
			},
			want: ocfl.ObjectState{
				Spec:  ocfl.Spec1_1,
				Flags: ocfl.HasNamaste,
			},
		},
		"extensions dir": {
			input: []fs.DirEntry{
				&dirEntry{name: objDecl},
				&dirEntry{name: "extensions", mode: fs.ModeDir},
			},
			want: ocfl.ObjectState{
				Spec:  ocfl.Spec1_1,
				Flags: ocfl.HasNamaste | ocfl.HasExtensions,
			},
		},
		"logs dir": {
			input: []fs.DirEntry{
				&dirEntry{name: objDecl},
				&dirEntry{name: "logs", mode: fs.ModeDir},
			},
			want: ocfl.ObjectState{
				Spec:  ocfl.Spec1_1,
				Flags: ocfl.HasNamaste | ocfl.HasLogs,
			},
		},
		"inventory and sidecar": {
			input: []fs.DirEntry{
				&dirEntry{name: objDecl},
				&dirEntry{name: "inventory.json"},
				&dirEntry{name: "inventory.json.sha256"},
			},
			want: ocfl.ObjectState{
				Spec:       ocfl.Spec1_1,
				Flags:      ocfl.HasNamaste | ocfl.HasInventory | ocfl.HasSidecar,
				SidecarAlg: "sha256",
			},
		},
		"irregular namaste": {
			input: []fs.DirEntry{
				&dirEntry{name: objDecl, mode: fs.ModeIrregular},
			},
			want: ocfl.ObjectState{
				Spec:  ocfl.Spec1_1,
				Flags: ocfl.HasNamaste,
			},
		},
		"symlink namaste": {
			input: []fs.DirEntry{
				&dirEntry{name: objDecl, mode: fs.ModeSymlink},
			},
			want: ocfl.ObjectState{
				Invalid: []string{"0=ocfl_object_1.1"},
			},
		},
	}
	i := 0
	for name, kase := range testCases {
		t.Run(fmt.Sprintf("case %d %s", i, name), func(t *testing.T) {
			got := ocfl.ParseObjectDir(kase.input)
			be.DeepEqual(t, kase.want, *got)
		})
		i++
	}

	// cases from fixtures
	type testCaseFixture struct {
		name string
		want ocfl.ObjectState
	}
	fixtureDir := filepath.Join(`testdata`, `object-fixtures`, `1.0`)
	fixtureCases := []testCaseFixture{
		{
			name: filepath.Join(`bad-objects`, `E001_extra_dir_in_root`),
			want: ocfl.ObjectState{
				Spec:        ocfl.Spec1_0,
				SidecarAlg:  "sha512",
				VersionDirs: []ocfl.VNum{ocfl.V(1)},
				Flags:       ocfl.HasInventory | ocfl.HasSidecar | ocfl.HasNamaste,
				Invalid:     []string{"extra_dir"},
			},
		}, {
			name: filepath.Join(`bad-objects`, `E001_v2_file_in_root`),
			want: ocfl.ObjectState{
				Spec:        ocfl.Spec1_0,
				SidecarAlg:  "sha512",
				VersionDirs: []ocfl.VNum{ocfl.V(1)},
				Flags:       ocfl.HasInventory | ocfl.HasSidecar | ocfl.HasNamaste,
				Invalid:     []string{"v2"},
			},
		}, {
			name: filepath.Join(`warn-objects`, `W013_unregistered_extension`),
			want: ocfl.ObjectState{
				Spec:        ocfl.Spec1_0,
				SidecarAlg:  "sha512",
				VersionDirs: []ocfl.VNum{ocfl.V(1)},
				Flags:       ocfl.HasInventory | ocfl.HasSidecar | ocfl.HasNamaste | ocfl.HasExtensions,
			},
		}, {
			name: filepath.Join(`bad-objects`, `E058_no_sidecar`),
			want: ocfl.ObjectState{
				Spec:        ocfl.Spec1_0,
				SidecarAlg:  "",
				VersionDirs: []ocfl.VNum{ocfl.V(1)},
				Flags:       ocfl.HasInventory | ocfl.HasNamaste,
			},
		}, {
			name: filepath.Join(`good-objects`, `minimal_with_logs`),
			want: ocfl.ObjectState{
				Spec:        ocfl.Spec1_0,
				SidecarAlg:  "sha512",
				VersionDirs: []ocfl.VNum{ocfl.V(1)},
				Flags:       ocfl.HasInventory | ocfl.HasNamaste | ocfl.HasLogs | ocfl.HasSidecar,
			},
		},
	}
	for _, fixCase := range fixtureCases {
		t.Run(fmt.Sprintf("fixture %s", fixCase.name), func(t *testing.T) {
			entries, err := os.ReadDir(filepath.Join(fixtureDir, fixCase.name))
			be.NilErr(t, err)
			got := ocfl.ParseObjectDir(entries)
			be.DeepEqual(t, fixCase.want, *got)
		})
	}
}

// dirEntry is used for testing object root parsing
type dirEntry struct {
	name string
	mode fs.FileMode
}

func (d dirEntry) Name() string      { return d.name }
func (d dirEntry) IsDir() bool       { return d.mode.IsDir() }
func (d dirEntry) Type() fs.FileMode { return d.mode.Type() }
func (d dirEntry) Info() (fs.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}
