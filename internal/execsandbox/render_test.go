package execsandbox

import (
	"reflect"
	"strings"
	"testing"
)

func TestSeatbeltProfile(t *testing.T) {
	base := Policy{
		Writable: []Path{{Path: "/work/repo", Kind: Dir}, {Path: "/private/tmp", Kind: Dir}},
		ReadOnly: []Path{{Path: "/work/repo/.git/hooks", Kind: Missing}},
		Hidden:   []Path{{Path: "/home/u/.ssh", Kind: Dir}},
	}
	withNetwork := base
	withNetwork.Network = true

	cases := []struct {
		name    string
		policy  Policy
		want    []string
		notWant []string
	}{
		{
			name:   "writes only to the writable subtrees",
			policy: base,
			want: []string{
				"(version 1)\n(allow default)\n(deny file-write*)\n",
				`(allow file-write* ` + seatbeltDeviceWrites + ` (subpath "/work/repo") (subpath "/private/tmp"))`,
			},
		},
		{
			name:   "read-only after writable, missing paths included",
			policy: base,
			want:   []string{`(deny file-write* (subpath "/work/repo/.git/hooks"))`},
		},
		{
			name:   "hidden paths: no read, no write, no socket connect",
			policy: base,
			want: []string{
				`(deny file-read* file-write* (subpath "/home/u/.ssh"))`,
				`(deny network-outbound (remote unix-socket (subpath "/home/u/.ssh")))`,
			},
		},
		{
			name:   "keychain services are always denied",
			policy: base,
			want:   []string{`(deny mach-lookup (global-name "com.apple.SecurityServer") (global-name "com.apple.securityd.xpc") (global-name "com.apple.security.keychaind"))`},
		},
		{
			name:   "no network: deny ip, keep loopback",
			policy: base,
			want:   []string{"(deny network-outbound (remote ip))\n(allow network-outbound (remote ip \"localhost:*\"))\n"},
		},
		{
			name:    "network allowed: no ip rules",
			policy:  withNetwork,
			notWant: []string{"(remote ip)"},
		},
		{
			name:    "empty lists render no empty rules",
			policy:  Policy{Network: true},
			want:    []string{`(allow file-write* ` + seatbeltDeviceWrites + `)`},
			notWant: []string{"(deny file-write*)\n(deny", "file-read*", "unix-socket"},
		},
		{
			name:   "quotes and backslashes are escaped",
			policy: Policy{Writable: []Path{{Path: `/odd/"dir"\x`}}, Network: true},
			want:   []string{`(subpath "/odd/\"dir\"\\x")`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SeatbeltProfile(tc.policy)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("profile lacks %q:\n%s", w, got)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got, w) {
					t.Errorf("profile has %q:\n%s", w, got)
				}
			}
		})
	}
}

func TestSeatbeltProfileOrdersPrecedence(t *testing.T) {
	got := SeatbeltProfile(Policy{
		Writable: []Path{{Path: "/w"}},
		ReadOnly: []Path{{Path: "/w/ro"}},
		Hidden:   []Path{{Path: "/w/secret"}},
	})
	// SBPL applies the last matching rule, so the order is the precedence.
	order := []string{"(deny file-write*)", "(allow file-write*", `(deny file-write* (subpath "/w/ro"))`, "(deny file-read* file-write*"}
	last := -1
	for _, marker := range order {
		i := strings.Index(got, marker)
		if i <= last {
			t.Fatalf("%q out of order in:\n%s", marker, got)
		}
		last = i
	}
}

func TestBubblewrapArgs(t *testing.T) {
	argv := []string{"/bin/sh", "-c", "make test"}
	cases := []struct {
		name   string
		policy Policy
		want   []string
	}{
		{
			name: "no network, every kind of path",
			policy: Policy{
				Writable: []Path{{Path: "/work", Kind: Dir}, {Path: "/cache/missing", Kind: Missing}},
				ReadOnly: []Path{{Path: "/work/.git/hooks", Kind: Dir}, {Path: "/work/.tars", Kind: Missing}},
				Hidden:   []Path{{Path: "/home/u/.ssh", Kind: Dir}, {Path: "/home/u/.netrc", Kind: File}, {Path: "/home/u/.aws", Kind: Missing}},
			},
			want: []string{
				"--die-with-parent", "--unshare-pid", "--new-session", "--unshare-net",
				"--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc",
				"--bind", "/work", "/work",
				"--ro-bind", "/work/.git/hooks", "/work/.git/hooks",
				"--tmpfs", "/home/u/.ssh", "--remount-ro", "/home/u/.ssh",
				"--ro-bind", "/dev/null", "/home/u/.netrc",
				"--chdir", "/work", "--", "/bin/sh", "-c", "make test",
			},
		},
		{
			name:   "network allowed keeps the host network",
			policy: Policy{Network: true},
			want: []string{
				"--die-with-parent", "--unshare-pid", "--new-session",
				"--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc",
				"--chdir", "/work", "--", "/bin/sh", "-c", "make test",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BubblewrapArgs(tc.policy, "/work", argv)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestBackendCommand(t *testing.T) {
	argv := []string{"/bin/sh", "-c", "true"}
	policy := Policy{Writable: []Path{{Path: "/w", Kind: Dir}}}

	name, args := Backend{Name: Seatbelt, Path: "/usr/bin/sandbox-exec"}.Command(policy, "/w", argv)
	if name != "/usr/bin/sandbox-exec" || len(args) != 5 || args[0] != "-p" || args[1] != SeatbeltProfile(policy) || !reflect.DeepEqual(args[2:], argv) {
		t.Fatalf("seatbelt command = %q %q", name, args)
	}

	name, args = Backend{Name: Bubblewrap, Path: "/usr/bin/bwrap"}.Command(policy, "/w", argv)
	if name != "/usr/bin/bwrap" || !reflect.DeepEqual(args, BubblewrapArgs(policy, "/w", argv)) {
		t.Fatalf("bwrap command = %q %q", name, args)
	}
}
