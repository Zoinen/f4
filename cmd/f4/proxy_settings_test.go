package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/plugins/netfox"
)

func TestProxySettings_SurviveSaveAndLoad(t *testing.T) {
	userIniPath := filepath.Join(t.TempDir(), "settings.ini")
	origUserPathFunc := getUserConfigIniPath
	origPathsFunc := getConfigIniPaths
	getUserConfigIniPath = func() string { return userIniPath }
	getConfigIniPaths = func() []string { return []string{userIniPath} }
	oldCfg := AppConfig
	oldProxy := netproxy.Global()
	t.Cleanup(func() {
		getUserConfigIniPath = origUserPathFunc
		getConfigIniPaths = origPathsFunc
		AppConfig = oldCfg
		netproxy.SetGlobal(oldProxy)
	})

	AppConfig.ProxyMode = netproxy.ModeHTTP
	AppConfig.ProxyHost = "gateway.local"
	AppConfig.ProxyPort = "8080"
	AppConfig.ProxyUser = "bob"
	AppConfig.ProxyPass = "s3cret"
	SaveConfig()

	// Saving publishes the settings, so a download started right after the
	// dialog closes already takes the new route.
	if got := netproxy.Global(); got.Mode != netproxy.ModeHTTP || got.Host != "gateway.local" || got.Pass != "s3cret" {
		t.Errorf("SaveConfig did not publish the proxy: %+v", got)
	}

	// The password is obfuscated on disk, like netfox site passwords.
	raw, err := os.ReadFile(userIniPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "[Proxy]") {
		t.Error("the ini has no [Proxy] section")
	}
	if strings.Contains(string(raw), "s3cret") {
		t.Error("the proxy password was written in the clear")
	}

	AppConfig.ProxyMode = netproxy.ModeSystem
	AppConfig.ProxyHost, AppConfig.ProxyPort, AppConfig.ProxyUser, AppConfig.ProxyPass = "", "", "", ""
	LoadConfig()

	if AppConfig.ProxyMode != netproxy.ModeHTTP || AppConfig.ProxyHost != "gateway.local" ||
		AppConfig.ProxyPort != "8080" || AppConfig.ProxyUser != "bob" || AppConfig.ProxyPass != "s3cret" {
		t.Errorf("proxy settings did not survive the round trip: %+v", AppConfig.ProxyMode)
	}
	if netproxy.Global().Host != "gateway.local" {
		t.Errorf("LoadConfig did not publish the proxy: %+v", netproxy.Global())
	}
}

func TestProxySettings_DefaultKeepsTheOldBehaviour(t *testing.T) {
	// Before this setting existed every download went through
	// http.DefaultClient, which honours the proxy environment variables.
	// A config file without a [Proxy] section must keep doing exactly that.
	userIniPath := filepath.Join(t.TempDir(), "settings.ini")
	if err := os.WriteFile(userIniPath, []byte("[Interface]\nColorStyle = Modern\n"), 0600); err != nil {
		t.Fatal(err)
	}
	origUserPathFunc := getUserConfigIniPath
	origPathsFunc := getConfigIniPaths
	getUserConfigIniPath = func() string { return userIniPath }
	getConfigIniPaths = func() []string { return []string{userIniPath} }
	oldCfg := AppConfig
	oldProxy := netproxy.Global()
	t.Cleanup(func() {
		getUserConfigIniPath = origUserPathFunc
		getConfigIniPaths = origPathsFunc
		AppConfig = oldCfg
		netproxy.SetGlobal(oldProxy)
	})

	AppConfig.ProxyMode = netproxy.ModeHTTP
	LoadConfig()
	if AppConfig.ProxyMode != netproxy.ModeSystem {
		t.Errorf("proxy mode without a [Proxy] section is %d, want ModeSystem", AppConfig.ProxyMode)
	}
}

func TestNetFoxConnectionOverridesTheGlobalProxy(t *testing.T) {
	old := netproxy.Global()
	t.Cleanup(func() { netproxy.SetGlobal(old) })
	netproxy.SetGlobal(netproxy.Settings{Mode: netproxy.ModeHTTP, Host: "corp", Port: "3128"})

	// A connection nobody touched follows f4.
	inherited := netfox.NetFoxConfig{Host: "files.example.com"}.Proxy()
	if inherited.Mode != netproxy.ModeHTTP || inherited.Host != "corp" {
		t.Errorf("a plain connection must inherit: %+v", inherited)
	}

	// One that carries its own settings does not.
	own := netfox.NetFoxConfig{
		Host:      "files.example.com",
		ProxyMode: netproxy.ModeSOCKS5,
		ProxyHost: "socks.example.com",
		ProxyUser: "bob",
		ProxyPass: "s3cret",
	}.Proxy()
	if own.Mode != netproxy.ModeSOCKS5 || own.Host != "socks.example.com" || own.Pass != "s3cret" {
		t.Errorf("an override must win: %+v", own)
	}
	if u := own.URL(); u == nil || u.Scheme != "socks5" || u.Host != "socks.example.com:1080" {
		t.Errorf("override url: %v", u)
	}

	// And one that opts out of proxying altogether stays direct.
	if direct := (netfox.NetFoxConfig{ProxyMode: netproxy.ModeDirect}).Proxy(); direct.Mode != netproxy.ModeDirect {
		t.Errorf("an explicit direct connection must stay direct: %+v", direct)
	}
}
