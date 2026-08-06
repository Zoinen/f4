package fishplus

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// HashEntry is one hashed file. The hash is whatever the remote tool
// produces, so it is only ever compared with another hash from the same
// host: two files are the same file if the same tool says so, which is all
// a duplicate search asks.
type HashEntry struct {
	Hash string
	Path string
}

// HashProgress reports how far the hashing has got. Total is how many files
// turned out to be worth hashing at all, which is known only after the tree
// has been walked, so it is not the number of files in it.
type HashProgress struct {
	Done  int
	Total int
	Path  string
}

// CanHash reports whether the remote host can hash a tree for us.
func (c *Client) CanHash() bool {
	feats := c.sess.Features()
	return c.CanRunJobs() && feats.HashTool() != "" &&
		feats.Has("findbin") && feats.Has("awk") && feats.ListingMode() != ""
}

// HashTool returns the hashing utility the helper found, announced as
// "hash:<name>", or the empty string when there is none.
func (f Features) HashTool() string {
	for name := range f.names {
		if strings.HasPrefix(name, "hash:") {
			return strings.TrimPrefix(name, "hash:")
		}
	}
	return ""
}

// StartHash starts a hashing job and returns its id.
func (c *Client) StartHash(ctx context.Context, dir string) (int, error) {
	if !c.CanHash() {
		return 0, ErrNoJobs
	}
	return c.StartJob(ctx, "hash", []string{dir})
}

// Hash hashes the files of a tree that could possibly have a twin and
// returns what it found. Files whose size occurs once in the tree are not
// hashed and not reported: they cannot be duplicates of anything, and
// reading them would be the expensive half of the job.
func (c *Client) Hash(ctx context.Context, dir string, cb func(HashProgress)) ([]HashEntry, error) {
	id, err := c.StartHash(ctx, dir)
	if err != nil {
		return nil, err
	}
	entries, err := c.FollowHash(ctx, id, cb)
	c.dropJobQuietly(id)
	return entries, err
}

// FollowHash polls a hashing job until it ends, on the same terms as
// FollowScan: the polls run on a context of their own so that cancelling is
// noticed between two of them rather than in the middle of one.
func (c *Client) FollowHash(ctx context.Context, id int, cb func(HashProgress)) ([]HashEntry, error) {
	reqCtx := context.WithoutCancel(ctx)
	var (
		entries  []HashEntry
		gotTotal bool
		wait     = jobPollMin
	)
	for {
		st, err := c.PollJob(reqCtx, id, DefaultPollLines)
		if err != nil {
			return entries, err
		}
		fresh := false
		for _, line := range st.Lines {
			switch {
			case strings.HasPrefix(line, "H "):
				hash, path, ok := strings.Cut(line[2:], " ")
				if !ok || hash == "" {
					continue
				}
				entries = append(entries, HashEntry{Hash: hash, Path: path})
				fresh = true
			case strings.HasPrefix(line, "P "):
				if cb == nil {
					continue
				}
				f := strings.SplitN(line[2:], " ", 3)
				if len(f) < 3 {
					continue
				}
				done, err1 := strconv.Atoi(f[0])
				total, err2 := strconv.Atoi(f[1])
				if err1 != nil || err2 != nil {
					continue
				}
				cb(HashProgress{Done: done, Total: total, Path: f[2]})
			case strings.HasPrefix(line, "T "):
				gotTotal = true
			}
		}
		if err := ctx.Err(); err != nil {
			return entries, err
		}
		if st.More {
			continue
		}
		switch st.State {
		case JobKilled:
			return entries, ErrJobKilled
		case JobDone:
			if st.Exit != 0 {
				msg := st.Msg
				if msg == "" {
					msg = "the remote hashing failed with status " + strconv.Itoa(st.Exit)
				}
				return entries, &RemoteError{Cmd: "hash", Msg: msg}
			}
			if !gotTotal {
				return entries, errors.New("fishplus: the remote hashing ended without a total")
			}
			return entries, nil
		}
		if fresh {
			wait = jobPollMin
		}
		if err := sleepCtx(ctx, wait); err != nil {
			return entries, err
		}
		wait *= 2
		if wait > jobPollMax {
			wait = jobPollMax
		}
	}
}

// Duplicates groups what Hash found by hash, keeping only the groups with
// more than one member. The order within a group is the order the remote
// walk reached them in, which is stable enough for a panel to show.
func (c *Client) Duplicates(ctx context.Context, dir string, cb func(HashProgress)) ([][]string, error) {
	entries, err := c.Hash(ctx, dir, cb)
	if err != nil {
		return nil, err
	}
	order := make([]string, 0, len(entries))
	byHash := make(map[string][]string, len(entries))
	for _, e := range entries {
		if _, seen := byHash[e.Hash]; !seen {
			order = append(order, e.Hash)
		}
		byHash[e.Hash] = append(byHash[e.Hash], e.Path)
	}
	groups := make([][]string, 0, len(order))
	for _, h := range order {
		if len(byHash[h]) > 1 {
			groups = append(groups, byHash[h])
		}
	}
	return groups, nil
}
