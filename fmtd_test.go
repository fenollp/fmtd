package fmtd_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"sort"
	"strings"
	"testing"

	"github.com/fenollp/fmtd"
	"github.com/stretchr/testify/require"
)

type tmpfiles map[string][]byte

func (fs tmpfiles) String() string {
	fns := fs.Filenames()
	sort.Strings(fns)
	return strings.Join(fns, "+")
}

func (fs tmpfiles) Filenames() []string {
	fns := make([]string, 0, len(fs))
	for fn := range fs {
		if fn != "" && strings.IndexByte("DLU", fn[0]) != -1 {
			fn = fn[1:]
		}
		fns = append(fns, fn)
	}
	return fns
}

func (fs tmpfiles) Unchanged(t *testing.T) {
	for fn, contents := range fs {
		switch {
		case contents == nil:
			continue
		case strings.IndexByte("DLU", fn[0]) != -1:
			continue
		}
		data, err := os.ReadFile(fn)
		require.NoError(t, err)
		require.Equal(t, string(data), string(contents))
	}
}

func maketmpfs(t *testing.T, fs map[string][]byte) func() {
	funcs := make([]func(), 0, len(fs))
	for filename, contents := range fs {
		filename, contents := filename, contents
		switch {
		case contents == nil:
			continue
		case filename[0] == 'D':
			continue
		case filename[0] == 'L':
			filename := filename[1:]
			err := os.Symlink(string(contents), filename)
			require.NoError(t, err)
			funcs = append(funcs, func() {
				err := os.Remove(filename)
				require.NoError(t, err)
			})
		case filename[0] == 'U':
			filename := filename[1:]
			err := os.WriteFile(filename, contents, 0400) // read-only
			require.NoError(t, err)
			funcs = append(funcs, func() {
				err := os.Remove(filename)
				require.NoError(t, err)
			})
		default:
			err := os.WriteFile(filename, contents, 0600) // write-only
			require.NoError(t, err)
			funcs = append(funcs, func() {
				err := os.Remove(filename)
				require.NoError(t, err)
			})
		}
	}
	return func() {
		for _, f := range funcs {
			f()
		}
	}
}

func TestFmtd(t *testing.T) {
	ctx := context.Background()
	pwd, err := os.Getwd()
	require.NoError(t, err)
	u, err := user.Current()
	require.NoError(t, err)
	HOME := u.HomeDir

	for _, fs := range []tmpfiles{
		// No files: don't fail but still check for usable docker client
		{},
		// Empty file name: fail
		{"": nil},
		// Non existing file: fail
		{"non-existing-file": nil},
		// Non existing file but another usable one: fail before formatting
		{"non-existing-file": nil, "testdata/some.json": []byte(`{ }`)},
		// A usable file but also a directory: fail before formatting
		{"testdata/some.json": []byte(`{ }`), "Dtestdata": nil},
		// A usable file but also a symlink: fail before formatting
		{"testdata/some.json": []byte(`{ }`), "Ltestdata/sym": []byte(`.gitkeep`)},
		// A usable file but also an unwritable one: fail before formatting
		{"testdata/some.json": []byte(`{ }`), "Utestdata/blip": []byte(`blop`)},
		// A usable file but also one above PWD: fail before formatting
		{"testdata/some.json": []byte(`{ }`), HOME + "/some_outside.yml": []byte(`bla:  42`)},
	} {
		for _, dryrun := range []bool{true, false} {
			name := fmt.Sprintf("_fns:%s_len:%d_dryrun:%v_", fs, len(fs), dryrun)
			t.Run(name, func(t *testing.T) {
				cleanup := maketmpfs(t, fs)
				defer cleanup()

				var stderr bytes.Buffer
				buf := io.MultiWriter(newTestingLogWriter(t), &stderr)
				err := fmtd.Fmt(ctx, pwd, dryrun, buf, fs.Filenames())
				switch {
				case len(fs.Filenames()) == 0:
					require.NoError(t, err)
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, "_fns:_len:1_"):
					require.EqualError(t, err, `unusable file "" (no such file or directory)`)
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, "non-existing-file"):
					require.EqualError(t, err, `unusable file "non-existing-file" (no such file or directory)`)
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, ":testdata+"):
					require.EqualError(t, err, `unusable file "testdata" (not a regular file)`)
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, "+testdata/sym_"):
					require.EqualError(t, err, `unusable file "testdata/sym" (not a regular file)`)
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, ":testdata/blip+"):
					if dryrun {
						require.NoError(t, err)
						require.NotEmpty(t, stderr.String())
					} else {
						require.EqualError(t, err, `unusable file "testdata/blip" (permission denied)`)
						require.Empty(t, stderr.String())
					}
					fs.Unchanged(t)

				case strings.Contains(name, HOME+"/some_outside.yml+"):
					require.EqualError(t, err, `unusable file "`+HOME+`/some_outside.yml" (not under $PWD)`)
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				default:
					panic(fmt.Sprintf("unhandled %s", fs))
				}
			})
		}
	}
}

// files not handled
// files already fmtd
// files
// dryrun files that need fmting

type tLogWriter struct {
	t *testing.T
}

func (tlw *tLogWriter) Write(p []byte) (n int, err error) {
	tlw.t.Logf("%s", p)
	return len(p), nil
}

func newTestingLogWriter(t *testing.T) io.Writer {
	return &tLogWriter{t}
}
