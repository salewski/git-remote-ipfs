package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cryptix/git-remote-ipfs/Godeps/_workspace/src/gopkg.in/errgo.v1"
)

// return the objects reachable from ref excluding the objects reachable from exclude
func gitListObjects(ref string, exclude []string) ([]string, error) {
	args := []string{"rev-list", "--objects", ref}
	for _, e := range exclude {
		args = append(args, "^"+e)
	}
	revList := exec.Command("git", args...)
	// dunno why - sometime git doesnt want to work on the inner repo/.git
	if strings.HasSuffix(thisGitRepo, ".git") {
		thisGitRepo = filepath.Dir(thisGitRepo)
	}
	revList.Dir = thisGitRepo // GIT_DIR
	out, err := revList.CombinedOutput()
	if err != nil {
		return nil, errgo.Notef(err, "rev-list failed: %s\n%q", err, string(out))
	}
	var objs []string
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		objs = append(objs, strings.Split(s.Text(), " ")[0])
	}
	if err := s.Err(); err != nil {
		return nil, errgo.Notef(err, "scanning rev-list output failed: %s", err)
	}
	return objs, nil
}

func gitFlattenObject(sha1 string) (io.Reader, error) {
	kind, err := gitCatKind(sha1)
	if err != nil {
		return nil, errgo.Notef(err, "flatten: kind(%s) failed", sha1)
	}
	size, err := gitCatSize(sha1)
	if err != nil {
		return nil, errgo.Notef(err, "flatten: size(%s) failed", sha1)
	}
	r, err := gitCatData(sha1, kind)
	if err != nil {
		return nil, errgo.Notef(err, "flatten: data(%s) failed", sha1)
	}
	// move to exp/git
	pr, pw := io.Pipe()
	go func() {
		zw := zlib.NewWriter(pw)
		if _, err := fmt.Fprintf(zw, "%s %d\x00", kind, size); err != nil {
			pw.CloseWithError(errgo.Notef(err, "writing git format header failed"))
			return
		}
		if _, err := io.Copy(zw, r); err != nil {
			pw.CloseWithError(errgo.Notef(err, "copying git data failed"))
			return
		}
		if err := zw.Close(); err != nil {
			pw.CloseWithError(errgo.Notef(err, "zlib close failed"))
			return
		}
		pw.Close()
	}()
	return pr, nil
}

func gitCatKind(sha1 string) (string, error) {
	catFile := exec.Command("git", "cat-file", "-t", sha1)
	catFile.Dir = thisGitRepo // GIT_DIR
	out, err := catFile.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func gitCatSize(sha1 string) (int64, error) {
	catFile := exec.Command("git", "cat-file", "-s", sha1)
	catFile.Dir = thisGitRepo // GIT_DIR
	out, err := catFile.CombinedOutput()
	if err != nil {
		return -1, errgo.Notef(err, "catSize(%s): run failed", sha1)
	}
	return strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
}

func gitCatData(sha1, kind string) (io.Reader, error) {
	catFile := exec.Command("git", "cat-file", kind, sha1)
	catFile.Dir = thisGitRepo // GIT_DIR
	stdout, err := catFile.StdoutPipe()
	if err != nil {
		return nil, errgo.Notef(err, "catData(%s): stdoutPipe failed", sha1)
	}
	stderr, err := catFile.StderrPipe()
	if err != nil {
		return nil, errgo.Notef(err, "catData(%s): stderrPipe failed", sha1)
	}
	r := io.MultiReader(stdout, stderr)
	if err := catFile.Start(); err != nil {
		out, err2 := ioutil.ReadAll(r)
		if err2 != nil {
			return nil, errgo.WithCausef(err2, err, "ReadAll failed")
		}
		return nil, errgo.Notef(err, "catData(%s) failed: %q", out)
	}
	// todo wait for cmd?!
	return r, nil
}

func gitRefHash(ref string) (string, error) {
	refParse := exec.Command("git", "rev-parse", ref)
	refParse.Dir = thisGitRepo // GIT_DIR
	out, err := refParse.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func gitIsAncestor(a, ref string) error {
	mergeBase := exec.Command("git", "merge-base", "--is-ancestor", a, ref)
	mergeBase.Dir = thisGitRepo // GIT_DIR
	if out, err := mergeBase.CombinedOutput(); err != nil {
		return errgo.Notef(err, "merge-base failed: %q", string(out))
	}
	return nil
}
