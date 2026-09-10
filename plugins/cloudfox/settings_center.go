package cloudfox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
	"golang.org/x/oauth2"
)

type centerSettingsProvider struct{ plugin *Plugin }

// Provider-owned JSON keys stay stable; unknown settings survive edits.
const cloudCenterFields = `
gdrive|client_id|OAuth client ID|OAuth audience used for desktop loopback authorization with PKCE.|string
gdrive|secret.client_secret|OAuth client secret|Client secret used during Google authorization. Leave blank to retain stored credentials.|secret
yandex|client_id|OAuth client ID|OAuth application used for browser authorization with PKCE.|string
yandex|root|Remote root|Root directory exposed by this connection, such as disk:/.|string
yandex|secret.oauth_token|OAuth token|Optional existing OAuth token. Leave blank to preserve stored credentials.|secret
s3|bucket|Bucket|Bucket name. Empty enables bucket discovery rather than selecting a bucket.|string
s3|region|Region|AWS region used for signing and endpoint selection. Default us-east-1.|string
s3|root_prefix|Root prefix|Object-key prefix exposed as the root of this connection.|string
s3|endpoint|Custom endpoint|Optional S3-compatible server endpoint instead of the standard AWS endpoint.|string
s3|auth|Authentication source|Use the default credential chain, an AWS profile, explicit static keys, or anonymous access.|choice
s3|profile|AWS profile|Named AWS credential profile used with profile authentication.|string
s3|secret.access_key_id|Access key ID|Static S3 access key. Entering static keys selects static authentication.|secret
s3|secret.secret_access_key|Secret access key|Static S3 signing secret. Excluded from search.|secret
s3|secret.session_token|Session token|Optional temporary-session credential for static S3 authentication.|secret
s3|custom_ca|Custom CA file|Additional certificate-authority file for the S3 endpoint.|path
s3|use_path_style|Use path-style addressing|Address buckets in the URL path instead of the endpoint hostname.|boolean
s3|allow_insecure|Allow credentials over HTTP|Permit explicit insecure S3 transport where the existing provider allows it.|boolean
webdav|base_url|Server URL|Base URL of the WebDAV server.|string
webdav|root|Remote root|Directory exposed as the root of this WebDAV connection.|string
webdav|auth|Authentication method|Use Basic, Digest, Bearer token, or anonymous authentication.|choice
webdav|username|Username|Account name for Basic or Digest authentication.|string
webdav|secret.password|Password|Account password. Leave blank to preserve stored credentials.|secret
webdav|secret.bearer_token|Bearer token|Token used with Bearer authentication. Leave blank to preserve stored credentials.|secret
webdav|custom_ca|Custom CA file|Additional certificate-authority file for the server.|path
webdav|allow_insecure_digest|Allow Digest over HTTP|Permit Digest authentication over unencrypted HTTP according to existing provider policy.|boolean
`

func (p *centerSettingsProvider) Catalog() f4settings.Catalog {
	c := f4settings.Catalog{ID: "cloudfox", Background: true, Categories: []f4settings.Category{{ID: "network", Label: f4settings.Text{English: "Network & connections"}}}}
	for _, provider := range []ProviderType{ProviderGoogleDrive, ProviderYandexDisk, ProviderS3, ProviderWebDAV} {
		prefix := "cloudfox." + string(provider) + "."
		group := providerLabel(provider) + " connections"
		field := func(key, label, desc string, kind f4settings.Kind) f4settings.Field {
			return f4settings.Scalar(prefix+key, "network", group, label, desc, kind)
		}
		fields := []f4settings.Field{field("name", "Connection name", "Display name of this saved connection.", f4settings.String), field("storage", "Secret storage", "Store credentials in the system keyring or encrypted vault. Portable profiles use the vault.", f4settings.ChoiceKind), field("credentials", "Credential changes", "Keep existing credentials, merge entered replacements, or explicitly clear all credentials. Opening Settings Center does not unlock stored secrets.", f4settings.ChoiceKind)}
		fields[1].Choices = f4settings.Choices("keyring:System keyring", "vault:Encrypted vault")
		if p.plugin.portable {
			fields[1].Choices = f4settings.Choices("vault:Encrypted vault")
		}
		fields[2].Choices = f4settings.Choices("keep:Keep existing; merge entered values", "replace:Replace complete credential bundle", "clear:Clear all credentials")
		for _, line := range strings.Split(strings.TrimSpace(cloudCenterFields), "\n") {
			parts := strings.Split(line, "|")
			if parts[0] != string(provider) {
				continue
			}
			f := field(parts[1], parts[2], parts[3], f4settings.Kind(parts[4]))
			switch parts[1] {
			case "client_id":
				if provider == ProviderGoogleDrive {
					f.Default = DefaultGoogleClientID
				} else {
					f.Default = DefaultYandexClientID
				}
			case "region":
				f.Default = "us-east-1"
			case "root":
				if provider == ProviderYandexDisk {
					f.Default = "disk:/"
				}
			case "auth":
				if provider == ProviderS3 {
					f.Choices = f4settings.Choices("default:Default credential chain", "profile:AWS profile", "static:Static keys", "anonymous:Anonymous")
				} else {
					f.Choices = f4settings.Choices("basic:Basic", "digest:Digest", "bearer:Bearer token", "anonymous:Anonymous")
				}
			}
			fields = append(fields, f)
		}
		col := f4settings.Collection{ID: "cloudfox." + string(provider), Category: "network", Group: group, Label: f4settings.Text{English: group}, Description: f4settings.Text{English: "Edit profile metadata inline. Secret values are not loaded until an explicit operation needs them. Authorize stages credentials; Apply saves the profile."}, NameField: prefix + "name", Fields: fields}
		col.Actions = []f4settings.RecordCommand{{ID: col.ID + ".authorize", Label: f4settings.Text{English: "Authorize / Test"}, Description: f4settings.Text{English: "Authorize this draft account or test its connection. Authorization results remain staged until Apply."}, Run: func(ctx context.Context, record f4settings.Record) (map[string]string, error) {
			return p.authorizeRecord(ctx, provider, record)
		}}}
		c.Collections = append(c.Collections, col)
	}
	return c
}

func cloudRecordDefaults(col f4settings.Collection) map[string]string {
	values := map[string]string{}
	for _, f := range col.Fields {
		value := f.Default
		if value == "" {
			if f.Kind == f4settings.Boolean {
				value = "false"
			}
			if len(f.Choices) > 0 {
				value = f.Choices[0].Value
			}
		}
		values[f.ID] = value
	}
	return values
}
func cloudRecord(col f4settings.Collection, connection Connection, id string) f4settings.Record {
	values := cloudRecordDefaults(col)
	prefix := col.ID + "."
	values[prefix+"name"] = connection.Name
	values[prefix+"__id"] = connection.ID
	values[prefix+"storage"] = "keyring"
	if strings.HasPrefix(connection.SecretRef, "vault:") {
		values[prefix+"storage"] = "vault"
	}
	var settings map[string]any
	_ = json.Unmarshal(connection.Settings, &settings)
	for key, value := range settings {
		values[prefix+key] = fmt.Sprint(value)
	}
	return f4settings.Record{ID: id, Revision: connection.UpdatedAt.Format(time.RFC3339Nano), Values: values}
}

func (p *centerSettingsProvider) connectionForRecord(provider ProviderType, col f4settings.Collection, r f4settings.Record, original *Connection) (Connection, SecretValues, error) {
	prefix := col.ID + "."
	c := Connection{Provider: provider}
	if original != nil {
		c = original.Clone()
	}
	c.Name = strings.TrimSpace(r.Values[prefix+"name"])
	settings := map[string]any{}
	if len(c.Settings) > 0 {
		if err := json.Unmarshal(c.Settings, &settings); err != nil {
			return c, nil, err
		}
	}
	typed := SecretValues{}
	for _, f := range col.Fields {
		key := strings.TrimPrefix(f.ID, prefix)
		value := r.Values[f.ID]
		if strings.HasPrefix(key, "secret.") {
			if value != "" {
				typed[strings.TrimPrefix(key, "secret.")] = value
			}
			continue
		}
		if key == "name" || key == "storage" || key == "credentials" {
			continue
		}
		if f.Kind == f4settings.Boolean {
			settings[key] = value == "true"
		} else {
			settings[key] = value
		}
	}
	if provider == ProviderS3 && (typed["access_key_id"] != "" || typed["secret_access_key"] != "" || typed["session_token"] != "") {
		settings["auth"] = "static"
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return c, nil, err
	}
	c.Settings = raw
	factory, ok := p.plugin.Factory(provider)
	if !ok {
		return c, nil, ErrFactoryNotRegistered
	}
	if err := validateConnection(c); err != nil {
		return c, nil, err
	}
	return c, typed, factory.Validate(c)
}

func (p *centerSettingsProvider) Begin(ctx context.Context) (*f4settings.Draft, error) {
	items, err := p.plugin.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	catalog := p.Catalog()
	records := map[string][]f4settings.Record{}
	originals := map[string]Connection{}
	for _, col := range catalog.Collections {
		records[col.ID] = nil
		for _, item := range items {
			if col.ID != "cloudfox."+string(item.Provider) {
				continue
			}
			records[col.ID] = append(records[col.ID], cloudRecord(col, item, item.ID))
			originals[item.ID] = item.Clone()
		}
	}
	d := f4settings.NewDraft(nil, records)
	d.ValidateFunc = func(d *f4settings.Draft) map[string]error {
		failures := map[string]error{}
		for _, col := range catalog.Collections {
			if !d.Dirty(col.ID) {
				continue
			}
			names := map[string]bool{}
			provider := ProviderType(strings.TrimPrefix(col.ID, "cloudfox."))
			for _, r := range d.Records[col.ID] {
				var original *Connection
				if old, ok := originals[r.ID]; ok {
					original = &old
				}
				candidate, _, err := p.connectionForRecord(provider, col, r, original)
				if err != nil {
					failures[r.ID] = err
				}
				name := strings.ToLower(candidate.Name)
				if names[name] {
					failures[r.ID] = ErrDuplicateName
				}
				names[name] = true
			}
		}
		return failures
	}
	d.CommitFunc = func(ctx context.Context, d *f4settings.Draft) f4settings.Result {
		result := f4settings.Result{Errors: map[string]error{}, Records: map[string]f4settings.Record{}}
		for _, col := range catalog.Collections {
			if !d.Dirty(col.ID) {
				continue
			}
			provider := ProviderType(strings.TrimPrefix(col.ID, "cloudfox."))
			prefix := col.ID + "."
			present := map[string]bool{}
			failed := false
			for _, r := range d.Records[col.ID] {
				present[r.ID] = true
				unchanged := false
				for _, base := range d.BaselineRecords[col.ID] {
					if base.ID == r.ID && reflect.DeepEqual(base, r) {
						unchanged = true
						break
					}
				}
				if unchanged {
					continue
				}
				if err := ctx.Err(); err != nil {
					result.Errors[r.ID] = err
					failed = true
					continue
				}
				var original *Connection
				if old, ok := originals[r.ID]; ok {
					original = &old
				}
				candidate, typed, err := p.connectionForRecord(provider, col, r, original)
				var saved Connection
				if err == nil {
					saved, err = p.saveRecord(ctx, candidate, original, r, prefix, typed)
				}
				if err != nil {
					result.Errors[r.ID] = err
					failed = true
					continue
				}
				originals[r.ID] = saved
				updated := cloudRecord(col, saved, r.ID)
				result.Records[r.ID] = updated
				result.Applied = append(result.Applied, r.ID)
			}
			for _, base := range d.BaselineRecords[col.ID] {
				if present[base.ID] {
					continue
				}
				original, ok := originals[base.ID]
				if !ok {
					continue
				}
				deleted, err := p.plugin.repo.Connections.DeleteIfCurrent(ctx, original)
				if err != nil {
					result.Errors[base.ID] = err
					failed = true
					continue
				}
				if deleted.SecretRef != "" {
					_ = p.plugin.repo.Secrets.Delete(ctx, deleted.SecretRef)
				}
				delete(originals, base.ID)
				result.Deleted = append(result.Deleted, base.ID)
			}
			if !failed {
				result.Applied = append(result.Applied, col.ID)
			}
		}
		return result
	}
	return d, nil
}

func (p *centerSettingsProvider) saveRecord(ctx context.Context, c Connection, original *Connection, r f4settings.Record, prefix string, typed SecretValues) (Connection, error) {
	storage := SecretStorage(r.Values[prefix+"storage"])
	if p.plugin.portable {
		storage = SecretStorageVault
	}
	var staged SecretValues
	if raw := r.Values[prefix+"__authorized"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &staged); err != nil {
			return c, err
		}
	}
	if c.Provider == ProviderGoogleDrive && len(staged) > 0 {
		var err error
		staged, err = googleStagedSecretsForConnection(c, staged, r.Values[prefix+"__audience"])
		if err != nil {
			return c, err
		}
	}
	scopeChanged, err := validateCredentialScopeChange(original, c, typed)
	if err != nil {
		return c, err
	}
	_, required, err := credentialScope(c)
	if err != nil {
		return c, err
	}
	mode := r.Values[prefix+"credentials"]
	storageChanged := original != nil && original.SecretRef != "" && ((storage == SecretStorageVault) != strings.HasPrefix(original.SecretRef, "vault:"))
	need := original == nil || len(typed) > 0 || len(staged) > 0 || storageChanged || scopeChanged || required || mode != "keep"
	var values SecretValues
	if need {
		values = SecretValues{}
		if original != nil && original.SecretRef != "" && !scopeChanged && mode == "keep" {
			values, err = p.plugin.repo.Credentials(ctx, *original)
			if isCredentialScopeBindingError(err) {
				err = validateRequiredSecrets(c, typed)
				values = SecretValues{}
			}
		}
		if err != nil {
			return c, err
		}
		if mode != "clear" {
			values = mergeSecretValues(values, staged)
			values = mergeSecretValues(values, typed)
			sanitizeSecretsForConnection(c, values)
			if err := validateRequiredSecrets(c, values); err != nil {
				return c, err
			}
		}
		defer clearSecrets(values)
		return p.plugin.repo.Save(ctx, c, &values, storage)
	}
	return p.plugin.repo.Save(ctx, c, nil, storage)
}

func (p *centerSettingsProvider) authorizeRecord(ctx context.Context, provider ProviderType, r f4settings.Record) (map[string]string, error) {
	var col f4settings.Collection
	for _, candidate := range p.Catalog().Collections {
		if candidate.ID == "cloudfox."+string(provider) {
			col = candidate
			break
		}
	}
	prefix := col.ID + "."
	var original *Connection
	if id := r.Values[prefix+"__id"]; id != "" {
		value, err := p.plugin.repo.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		original = &value
	}
	c, typed, err := p.connectionForRecord(provider, col, r, original)
	if err != nil {
		return nil, err
	}
	stored := SecretValues{}
	if original != nil && original.SecretRef != "" {
		stored, err = p.plugin.repo.Credentials(ctx, *original)
		if err != nil && !isCredentialScopeBindingError(err) {
			return nil, err
		}
	}
	defer clearSecrets(stored)
	switch provider {
	case ProviderGoogleDrive:
		audienceChanged := false
		if original != nil {
			oldID, _ := googleClientID(*original)
			newID, _ := googleClientID(c)
			audienceChanged = oldID != newID
		}
		stored = googleAuthorizationSecrets(stored, typed, audienceChanged)
		authCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		stored, err = AuthorizeGoogleDesktop(authCtx, c, stored, nil)
	case ProviderYandexDisk:
		var settings YandexDiskSettings
		_ = json.Unmarshal(c.Settings, &settings)
		verifier := oauth2.GenerateVerifier()
		url, urlErr := YandexAuthorizationURL("", settings.ClientID, "", verifier)
		err = urlErr
		if err == nil {
			err = openBrowserURL(url)
		}
		task, ok := ctx.(*vtui.TaskContext)
		if !ok {
			return nil, f4settings.Error("authorization requires interactive task context")
		}
		var code string
		if err == nil {
			code, err = promptYandexAuthorizationCode(task)
		}
		if err == nil {
			stored, err = ExchangeYandexAuthorizationCode(ctx, http.DefaultClient, "", settings.ClientID, code, verifier)
		}
	default:
		scopeChanged, scopeErr := validateCredentialScopeChange(original, c, typed)
		if scopeErr != nil {
			return nil, scopeErr
		}
		if scopeChanged {
			stored = SecretValues{}
		}
		stored = mergeSecretValues(stored, typed)
		sanitizeSecretsForConnection(c, stored)
		if err := validateRequiredSecrets(c, stored); err != nil {
			return nil, err
		}
		factory, ok := p.plugin.Factory(provider)
		if !ok {
			return nil, ErrFactoryNotRegistered
		}
		backend, err := factory.Open(ctx, c, stored)
		if err != nil {
			return nil, err
		}
		defer func() { _ = backend.Close() }() // Authentication result is already handled; release the temporary backend.
		return nil, testCloudBackend(ctx, backend)
	}
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return nil, err
	}
	audience, _ := googleClientID(c)
	return map[string]string{prefix + "__authorized": string(raw), prefix + "__audience": audience}, nil
}
