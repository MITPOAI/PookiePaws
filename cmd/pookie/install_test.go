package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withGithubAPIBase swaps the package-level GitHub API base URL for the
// duration of the test, restoring it afterwards.
func withGithubAPIBase(t *testing.T, base string) {
	t.Helper()
	prev := githubAPIBase
	githubAPIBase = base
	t.Cleanup(func() { githubAPIBase = prev })
}

func TestResolveLatestTagPicksHighest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/repos/acme/widget/tags") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name": "v1.0.0"},
			{"name": "v1.2.3"},
			{"name": "v1.2.0"},
			{"name": "foo"},
			{"name": "0.9.0"}
		]`))
	}))
	defer srv.Close()
	withGithubAPIBase(t, srv.URL)

	got := resolveLatestTag("acme", "widget")
	if got != "v1.2.3" {
		t.Fatalf("resolveLatestTag = %q, want v1.2.3", got)
	}
}

func TestResolveLatestTagInvalidJSONReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json at all`))
	}))
	defer srv.Close()
	withGithubAPIBase(t, srv.URL)

	if got := resolveLatestTag("acme", "widget"); got != "" {
		t.Fatalf("resolveLatestTag with bad JSON = %q, want empty", got)
	}
}

func TestResolveLatestTagNoTagsReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	withGithubAPIBase(t, srv.URL)

	if got := resolveLatestTag("acme", "widget"); got != "" {
		t.Fatalf("resolveLatestTag with empty tag list = %q, want empty", got)
	}
}

func TestResolveLatestTagNonOKReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	withGithubAPIBase(t, srv.URL)

	if got := resolveLatestTag("acme", "widget"); got != "" {
		t.Fatalf("resolveLatestTag on 403 = %q, want empty", got)
	}
}

func TestResolveLatestTagSkipsNonSemverTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"name": "release-candidate"},
			{"name": "nightly"},
			{"name": "v0.1.0"}
		]`))
	}))
	defer srv.Close()
	withGithubAPIBase(t, srv.URL)

	if got := resolveLatestTag("acme", "widget"); got != "v0.1.0" {
		t.Fatalf("resolveLatestTag = %q, want v0.1.0", got)
	}
}

func TestConfirmYesNoEmptyInputDeniesByDefault(t *testing.T) {
	if confirmYesNo("Proceed?", strings.NewReader("\n")) {
		t.Fatal("empty input should deny")
	}
}

func TestConfirmYesNoEOFDenies(t *testing.T) {
	if confirmYesNo("Proceed?", strings.NewReader("")) {
		t.Fatal("EOF should deny")
	}
}

func TestConfirmYesNoAcceptsYAndYes(t *testing.T) {
	for _, in := range []string{"y\n", "Y\n", "yes\n", "YES\n", "  yes  \n"} {
		if !confirmYesNo("Proceed?", strings.NewReader(in)) {
			t.Fatalf("input %q should be accepted", in)
		}
	}
}

func TestConfirmYesNoRejectsOtherInput(t *testing.T) {
	for _, in := range []string{"n\n", "no\n", "maybe\n", "yep\n", "1\n"} {
		if confirmYesNo("Proceed?", strings.NewReader(in)) {
			t.Fatalf("input %q should be rejected", in)
		}
	}
}

func TestParseRepoArgWithLatest(t *testing.T) {
	owner, repo, ref, sub := parseRepoArg("acme/widget@latest")
	if owner != "acme" || repo != "widget" || ref != "latest" || sub != "" {
		t.Fatalf("parseRepoArg(acme/widget@latest) = (%q,%q,%q,%q)", owner, repo, ref, sub)
	}
}
