package panel

import "testing"

// f4#1924: the title of a panel writes the home directory as "~", like the
// prompt of the command line, and leaves a directory that only shares the
// prefix of the home as it is.
func TestHomeTildePath(t *testing.T) {
	for _, c := range []struct{ path, home, want string }{
		{"/home/user", "/home/user", "~"},
		{"/home/user/bla-bla", "/home/user", "~/bla-bla"},
		{"/home/user/bla-bla", "/home/user/", "~/bla-bla"},
		{"/home/user2/x", "/home/user", "/home/user2/x"},
		{"/tmp", "/home/user", "/tmp"},
		{"/home/user", "", "/home/user"},
	} {
		if got := homeTildePath(c.path, c.home); got != c.want {
			t.Errorf("homeTildePath(%q, %q) = %q, want %q", c.path, c.home, got, c.want)
		}
	}
}
