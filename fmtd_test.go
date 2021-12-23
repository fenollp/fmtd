package fmtd_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"regexp"
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

func (fs tmpfiles) Changed(t *testing.T) {
	var fnFormatted, fnUnformatted string
	for fn := range fs {
		if strings.Contains(fn, "/formatted.") {
			fnFormatted = fn
		}
		if strings.Contains(fn, "/unformatted.") {
			fnUnformatted = fn
		}
	}
	require.NotEqual(t, string(fs[fnFormatted]), string(fs[fnUnformatted]))
	data, err := os.ReadFile(fnUnformatted)
	require.NoError(t, err)
	require.Equal(t, string(data), string(fs[fnFormatted]))
}

func maketmpfs(t *testing.T, fs tmpfiles) func() {
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

	if _, ok := fs["testdata/sets_arg.go"]; ok {
		err := os.Setenv("ARG_GOFMT_IMAGE", "docker.io/library/hello-world")
		require.NoError(t, err)
		funcs = append(funcs, func() {
			err := os.Unsetenv("ARG_GOFMT_IMAGE")
			require.NoError(t, err)
		})
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
		{"non-existing-file": nil, "testdata/some.json": []byte("{ }")},
		// A usable file but also a directory: fail before formatting
		{"testdata/some.json": []byte("{ }"), "Dtestdata": nil},
		// A usable file but also a symlink: fail before formatting
		{"testdata/some.json": []byte("{ }"), "Ltestdata/sym": []byte(".gitkeep")},
		// A usable file but also an unwritable one: fail before formatting
		{"testdata/some.json": []byte("{ }"), "Utestdata/blip": []byte("blop")},
		// A usable file but also one above PWD: fail before formatting
		{"testdata/some.json": []byte("{ }"), HOME + "/some_outside.yml": []byte("bla:  42")},
		// Unhandled file: show a warning
		{"testdata/some.xyz": []byte("bla")},
		// A Go file using ARG_...: runtime failure
		{"testdata/sets_arg.go": []byte("package    bla")},
		// A formatted and an unformatted file: JSON
		{"testdata/formatted.json": []byte("{}\n"), "testdata/unformatted.json": []byte("{ }")},
	} {
		for _, dryrun := range []bool{true, false} {
			name := fmt.Sprintf("_fns:%s_len:%d_dryrun:%v_", fs, len(fs), dryrun)
			t.Run(name, func(t *testing.T) {
				cleanup := maketmpfs(t, fs)
				defer cleanup()

				var stdout, stderr bytes.Buffer
				bufout := io.MultiWriter(newTestingLogWriter(t, "STDOUT"), &stdout)
				buferr := io.MultiWriter(newTestingLogWriter(t, "STDERR"), &stderr)
				err := fmtd.Fmt(ctx, pwd, dryrun, bufout, buferr, fs.Filenames())
				switch {
				case len(fs.Filenames()) == 0:
					require.NoError(t, err)
					require.Empty(t, stdout.String())
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, "_fns:_len:1_"):
					require.EqualError(t, err, `unusable file "" (no such file or directory)`)
					require.Empty(t, stdout.String())
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, "non-existing-file"):
					require.EqualError(t, err, `unusable file "non-existing-file" (no such file or directory)`)
					require.Empty(t, stdout.String())
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, ":testdata+"):
					require.EqualError(t, err, `unusable file "testdata" (not a regular file)`)
					require.Empty(t, stdout.String())
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, "+testdata/sym_"):
					require.EqualError(t, err, `unusable file "testdata/sym" (not a regular file)`)
					require.Empty(t, stdout.String())
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, ":testdata/blip+"):
					if dryrun {
						require.EqualError(t, err, fmtd.ErrDryRunFoundFiles.Error())
						require.Contains(t, stdout.String(), `testdata/some.json`)
						require.NotRegexp(t, regexp.MustCompile("^testdata/blip$"), stdout.String())
						require.Contains(t, stdout.String(), `! testdata/blip`)
						require.NotEmpty(t, stderr.String())
					} else {
						require.EqualError(t, err, `unusable file "testdata/blip" (permission denied)`)
						require.Empty(t, stdout.String())
						require.Empty(t, stderr.String())
					}
					fs.Unchanged(t)

				case strings.Contains(name, HOME+"/some_outside.yml+"):
					require.EqualError(t, err, `unusable file "`+HOME+`/some_outside.yml" (not under $PWD)`)
					require.Empty(t, stdout.String())
					require.Empty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, "some.xyz"):
					require.NoError(t, err)
					require.Contains(t, stdout.String(), `! testdata/some.xyz`)
					require.NotEmpty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, "/sets_arg."):
					require.EqualError(t, err, fmtd.ErrDockerBuildFailure.Error())
					require.Empty(t, stdout.String())
					require.NotEmpty(t, stderr.String())
					fs.Unchanged(t)

				case strings.Contains(name, "formatted.json"):
					if dryrun {
						require.EqualError(t, err, fmtd.ErrDryRunFoundFiles.Error())
						fs.Unchanged(t)
					} else {
						require.NoError(t, err)
						fs.Changed(t)
					}
					require.Contains(t, stdout.String(), `testdata/unformatted.json`)
					require.NotContains(t, stdout.String(), `testdata/formatted.json`)
					require.NotEmpty(t, stderr.String())

				default:
					panic(fmt.Sprintf("unhandled %s", fs))
				}
			})
		}
	}
}

type tLogWriter struct {
	prefix string
	t      *testing.T
}

func (tlw *tLogWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	for _, pp := range bytes.Split(p, []byte{'\r', '\n'}) {
		for _, ppp := range bytes.Split(pp, []byte{'\n'}) {
			if pppp := bytes.TrimSpace(ppp); len(pppp) != 0 {
				tlw.t.Logf("%s %s", tlw.prefix, pppp)
			}
		}
	}
	return
}

func newTestingLogWriter(t *testing.T, prefix string) io.Writer {
	return &tLogWriter{prefix, t}
}
