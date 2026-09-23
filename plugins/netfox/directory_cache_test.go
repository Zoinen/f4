package netfox

import "testing"

func TestNetFoxDirectoryCacheIdentitySurvivesTransportFallback(t *testing.T) {
	cfg := NetFoxConfig{
		Host:     "202.61.255.60",
		Port:     "22",
		User:     "root",
		KeyPath:  "/Users/zoin/.ssh/id_ed25519",
		Codepage: "65001",
		Timeout:  "15",
		Options:  map[string]string{"Passive": "true", "Encoding": "utf-8"},
	}

	// An OpenSSH profile can first try SFTP and then fall back to FISH+.
	// Both views must address the same cached directory snapshot.
	sftpFallback := netFoxDirectoryCacheIdentity("de_zoin", cfg)
	cfg.Type = "fish+"
	fishExplicit := netFoxDirectoryCacheIdentity("de_zoin", cfg)
	if sftpFallback == fishExplicit {
		t.Fatal("explicit FISH+ and SFTP profile unexpectedly share a cache identity")
	}
	cfg.Type = ""
	secondOpenSSHView := netFoxDirectoryCacheIdentity("de_zoin", cfg)
	if sftpFallback != secondOpenSSHView {
		t.Fatalf("same OpenSSH profile changed cache identity: %q != %q", sftpFallback, secondOpenSSHView)
	}

	// Password rotation does not change the remote file identity and must not
	// make a useful provisional listing disappear during the next login.
	cfg.Pass = "rotated-secret"
	if got := netFoxDirectoryCacheIdentity("de_zoin", cfg); got != sftpFallback {
		t.Fatalf("password rotation changed cache identity: %q != %q", got, sftpFallback)
	}
}

func TestNetFoxDirectoryCacheIdentityChangesForDifferentRemote(t *testing.T) {
	base := NetFoxConfig{Host: "host-a", Port: "22", User: "root"}
	want := netFoxDirectoryCacheIdentity("profile", base)
	for name, mutate := range map[string]func(*NetFoxConfig){
		"host":     func(cfg *NetFoxConfig) { cfg.Host = "host-b" },
		"user":     func(cfg *NetFoxConfig) { cfg.User = "other" },
		"port":     func(cfg *NetFoxConfig) { cfg.Port = "2222" },
		"protocol": func(cfg *NetFoxConfig) { cfg.Type = "ftp" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := base
			mutate(&cfg)
			if got := netFoxDirectoryCacheIdentity("profile", cfg); got == want {
				t.Fatalf("changed %s did not change cache identity %q", name, got)
			}
		})
	}
}
