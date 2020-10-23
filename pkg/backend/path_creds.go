package backend

import (
	"context"
	"crypto/sha1"
	"fmt"
	"strings"

	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
	"golang.org/x/oauth2"
)

const (
	credsPath       = "creds"
	credsPathPrefix = credsPath + "/"
)

// credKey hashes the name and splits the first few bytes into separate buckets
// for performance reasons.
func credKey(name string) string {
	hash := sha1.Sum([]byte(name))
	first, second, rest := hash[:2], hash[2:4], hash[4:]
	return credsPathPrefix + fmt.Sprintf("%x/%x/%x", first, second, rest)
}

func (b *backend) credsReadOperation(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	key := credKey(data.Get("name").(string))

	var (
		tok *oauth2.Token
		err error
	)

	if scopes, ok := data.GetOk("scopes"); ok {
		tok, err = b.getRefreshToken(ctx, req.Storage, key, scopes.([]string), false)
	} else {
		tok, err = b.getRefreshToken(ctx, req.Storage, key, nil, false)
	}

	if err == ErrNotConfigured {
		return logical.ErrorResponse("not configured"), nil
	} else if err == ErrInvalidCredentials {
		return logical.ErrorResponse("invalid client credentials"), nil
	} else if err != nil {
		return nil, err
	} else if tok == nil {
		return nil, nil
	} else if !tok.Valid() {
		return logical.ErrorResponse("token expired"), nil
	}

	rd := map[string]interface{}{
		"access_token": tok.AccessToken,
		"type":         tok.Type(),
	}

	if !tok.Expiry.IsZero() {
		rd["expire_time"] = tok.Expiry
	}

	resp := &logical.Response{
		Data: rd,
	}
	return resp, nil
}

func (b *backend) credsUpdateOperation(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	c, err := getConfig(ctx, req.Storage)
	if err != nil {
		return nil, err
	} else if c == nil {
		return logical.ErrorResponse("not configured"), nil
	}

	p, err := c.provider(b.providerRegistry)
	if err != nil {
		return nil, err
	}

	if !p.IsAuthorizationRequired() {
		return logical.ErrorResponse(`this provider does not support creating credentials. Use "read" command to retrieve token`), nil
	}

	key := credKey(data.Get("name").(string))

	var tok *oauth2.Token

	cb := p.NewExchangeConfigBuilder(c.ClientID, c.ClientSecret)
	if code, ok := data.GetOk("code"); ok {
		if _, ok := data.GetOk("refresh_token"); ok {
			return logical.ErrorResponse("cannot use both code and refresh_token"), nil
		}

		if redirectURL, ok := data.GetOk("redirect_url"); ok {
			cb = cb.WithRedirectURL(redirectURL.(string))
		}

		tok, err = cb.Build().Exchange(ctx, code.(string))
		if rErr, ok := err.(*oauth2.RetrieveError); ok {
			b.logger.Error("invalid code", "error", rErr)
			return logical.ErrorResponse("invalid code"), nil
		} else if err != nil {
			return nil, err
		}
	} else if refreshToken, ok := data.GetOk("refresh_token"); ok {
		tok, err = cb.Build().TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken.(string)}).Token()
		if rErr, ok := err.(*oauth2.RetrieveError); ok {
			b.logger.Error("invalid refresh_token", "error", rErr)
			return logical.ErrorResponse("invalid refresh_token"), nil
		} else if err != nil {
			return nil, err
		}
		// tok now contains a refresh token and an access token
	} else {
		return logical.ErrorResponse("missing code or refresh_token"), nil
	}

	b.credMut.Lock()
	defer b.credMut.Unlock()

	// TODO: Handle extra fields?
	entry, err := logical.StorageEntryJSON(key, tok)
	if err != nil {
		return nil, err
	}

	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, err
	}

	return nil, nil
}

func (b *backend) credsDeleteOperation(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	b.credMut.Lock()
	defer b.credMut.Unlock()

	key := credKey(data.Get("name").(string))

	if err := req.Storage.Delete(ctx, key); err != nil {
		return nil, err
	}

	return nil, nil
}

var credsFields = map[string]*framework.FieldSchema{
	"name": {
		Type:        framework.TypeString,
		Description: "Specifies the name of the credential.",
	},
	"code": {
		Type:        framework.TypeString,
		Description: "Specifies the response code to exchange for a full token.",
	},
	"redirect_url": {
		Type:        framework.TypeString,
		Description: "Specifies the redirect URL to provide when exchanging (required by some services and must be equivalent to the redirect URL provided to the authorization code URL).",
	},
	"refresh_token": {
		Type:        framework.TypeString,
		Description: "Specifies a refresh token retrieved from the provider by some means external to this plugin.",
	},
}

// Allow characters not special to urls or shells
// Derived from framework.GenericNameWithAtRegex
func credentialNameRegex(name string) string {
	return fmt.Sprintf(`(?P<%s>\w(([\w.@~!_,:^-]+)?\w)?)`, name)
}

const credsHelpSynopsis = `
Provides access tokens for authorized credentials.
`

const credsHelpDescription = `
This endpoint allows users to configure credentials to the service.
Write a credential to this endpoint by specifying the code from the
HTTP response of the authorization redirect. If the code is valid,
the access token will be available when reading the endpoint.
`

func pathCreds(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: credsPathPrefix + credentialNameRegex("name") + `$`,
		Fields:  credsFields,
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ReadOperation: &framework.PathOperation{
				Callback: b.credsReadOperation,
				Summary:  "Get a current access token for this credential.",
			},
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.credsUpdateOperation,
				Summary:  "Write a new credential or update an existing credential.",
			},
			logical.DeleteOperation: &framework.PathOperation{
				Callback: b.credsDeleteOperation,
				Summary:  "Remove a credential.",
			},
		},
		HelpSynopsis:    strings.TrimSpace(credsHelpSynopsis),
		HelpDescription: strings.TrimSpace(credsHelpDescription),
	}
}
