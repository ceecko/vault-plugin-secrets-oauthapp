# vault-plugin-secrets-oauthapp

This is a standalone backend plugin for use with [Hashicorp
Vault](https://www.github.com/hashicorp/vault).

This plugin provides a secure wrapper around OAuth 2 authorization code grant
flows, allowing a Vault client to request authorization on behalf of a user and
perform actions using a negotiated OAuth 2 access token.

## Usage

Once you have the binary, you will need to register the plugin with Vault.
Follow [the instructions in the Vault
documentation](https://www.vaultproject.io/docs/internals/plugins.html#plugin-registration)
to add the plugin to the catalog. We will assume it is registered under the name
`oauthapp`.

Mount the plugin at the path of your choosing:

```console
$ vault secrets enable -path=oauth2/bitbucket oauthapp
Success! Enabled the oauthapp secrets engine at: oauth2/bitbucket/
```

### OAuth with authorization (3-legged OAuth)
Configure it with the necessary information to exchange tokens:

```console
$ vault write oauth2/bitbucket/config \
    provider=bitbucket \
    client_id=aBcD3FgHiJkLmN0pQ \
    client_secret=AbCd3fGh1jK1MnoPqRs7uVwXYz
Success! Data written to: oauth2/bitbucket/config
```

Once the client secret has been written, it will never be exposed again.

From a Vault client, request an authorization code URL:

```console
$ vault write oauth2/bitbucket/config/auth_code_url state=foo scopes=bar,baz
Key    Value
---    -----
url    https://bitbucket.org/site/oauth2/authorize?client_id=aBcD3FgHiJkLmN0pQ&response_type=code&scope=bar+baz&state=foo
```

After redirecting the user to that URL and receiving the resulting temporary
authorization code in your callback handler, you can create a permanent
credential that automatically refreshes:

```console
$ vault write oauth2/bitbucket/creds/my-user-auth code=zYxWvU7sRqP
Success! Data written to: oauth2/bitbucket/creds/my-user-auth
```

Assuming the refresh token remains valid, an access token is available any time at the same endpoint:

```console
$ vault read oauth2/bitbucket/creds/my-user-auth
Key             Value
---             -----
access_token    nLlBg9Lmd7n1X96bw/xcW9HvyOHzxj19z3zXKv0XXxr8eLjQSerf4iyPDRCucSHQN+c7fnKhPsSWbWg0
```

Note that the client secret and refresh token are never exposed to Vault
clients.

Alternatively, if a refresh token is obtained in some other way you can
skip the auth_code_url step and pass the token directly to the creds
write instead of the response code:

```console
$ vault write oauth2/bitbucket/creds/my-user-tokens refresh_token=eyJhbGciOiJub25lIn0.eyJqdGkiOiI4YTE0NDRjMi0wYjQxLTRiZDUtODI3My0yZDBkMDczODQ4MDAifQ.
Success! Data written to: oauth2/bitbucket/creds/my-user-tokens
```

### OAuth without authorization (2-legged OAuth)
Configure it with the necessary information to exchange tokens:

```console
$ vault write oauth2/custom_client_credentials/config \
    provider=custom_client_credentials \
    client_id=aBcD3FgHiJkLmN0pQ \
    client_secret=AbCd3fGh1jK1MnoPqRs7uVwXYz \
    '{"provider_options": {"token_url": "https://url-to-provider/token"}}'
Success! Data written to: oauth2/custom_client_credentials/config
```

Once the client secret has been written, it will never be exposed again.

Get token. Once the token is expired it will be renewed automatically
with the next read.

```console
$ vault read oauth2/custom_client_credentials/creds/my-user-auth
Key             Value
---             -----
access_token    nLlBg9Lmd7n1X96bw/xcW9HvyOHzxj19z3zXKv0XXxr8eLjQSerf4iyPDRCucSHQN+c7fnKhPsSWbWg0
```

Note that the client secret is never exposed to Vault clients.


## Endpoints

### `config`

#### `GET` (`read`)

Retrieve the current configuration settings (except the client secret).

#### `PUT` (`write`)

Write new configuration settings. This endpoint completely replaces the existing
configuration, so you must specify all required fields, even when updating.

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|----------|
| `client_id` | The OAuth 2.0 client ID. | String | None | Yes |
| `client_secret` | The OAuth 2.0 client secret. | String | None | Yes |
| `auth_url_params` | A map of additional query string parameters to provide to the authorization code URL. | Map of String🠦String | None | No |
| `provider` | The name of the provider to use. See [the list of providers](#providers). | String | None | Yes |
| `provider_options` | Options to configure the specified provider. | Map of String🠦String | None | No |

#### `DELETE` (`delete`)

Remove the current configuration. This does not invalidate any existing access
tokens; however, you will not be able to create new tokens or refresh existing
tokens.

### `config/auth_code_url`

#### `PUT` (`write`)

Retrieve an authorization code URL for the given state.  
Not supported with 2-legged OAuth.

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|----------|
| `auth_url_params` | A map of additional query string parameters to provide to the authorization code URL. If any keys in this map conflict with the parameters stored in the configuration, the configuration's parameters take precedence. | Map of String🠦String | None | No |
| `redirect_url` | The URL to redirect to once the user has authorized this application. | String | None | No |
| `scopes` | A list of explicit scopes to request. | List of String | None | No |
| `state` | The unique state to send to the authorization URL. | String | None | Yes |

### `creds/:name`

#### `GET` (`read`)

Retrieve a current access token for the given credential.

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|----------|
| `scopes` | A list of explicit scopes to request for 2-legged OAuth. Has no meaning for 3-legged OAuth. | List of String | None | No |

#### `PUT` (`write`)

Create or update a credential after an authorization flow has returned to the
application.  
Not supported with 2-legged OAuth.

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|----------|
| `code` | The response code to exchange for a full token. | String | None | Either this or `refresh_token` |
| `redirect_url` | The same redirect URL as specified in the authorization code URL. | String | None | Refer to provider documentation |
| `refresh_token` | A refresh token retrieved from the provider by some means external to this plugin. | String | None | Either this or `code` |

#### `DELETE` (`delete`)

Remove the credential information from storage.

## Providers

### Bitbucket (`bitbucket`)

[Documentation](https://developer.atlassian.com/cloud/bitbucket/oauth-2/)

### GitHub (`github`)

[Documentation](https://developer.github.com/apps/building-oauth-apps/authorizing-oauth-apps/)

### GitLab (`gitlab`)

[Documentation](https://docs.gitlab.com/ee/api/oauth2.html)

### Google (`google`)

[Documentation](https://developers.google.com/identity/protocols/oauth2)

### Microsoft Azure AD (`microsoft_azure_ad`)

[Documentation](https://docs.microsoft.com/en-us/azure/active-directory/develop/v2-oauth2-auth-code-flow)

#### Options

| Name | Description | Default | Required |
|------|-------------|---------|----------|
| `tenant` | The tenant to authenticate to. | None | Yes |

### Slack (`slack`)

[Documentation](https://api.slack.com/docs/oauth)

### Custom (`custom`)

This provider allows you to specify the required endpoints for negotiating an
arbitrary OAuth 2 authorization code grant flow.

#### Options

| Name | Description | Default | Required |
|------|-------------|---------|----------|
| `discovery_url` | The URL at which to discover authorization code and token URLs. | None | Either this or the other two urls |
| `auth_code_url` | The URL to submit the initial authorization code request to. | None | Either this or `discovery_url` |
| `token_url` | The URL to use for exchanging temporary codes and refreshing access tokens. | None | Either this or `discovery_url` |
| `auth_style` | How to authenticate to the token URL. If specified, must be one of `in_header` or `in_params`. | Automatically detect | No |

### Custom with no authorization - 2-legged OAuth (`custom_client_credentials`)

This provider allows you to specify the required endpoints for negotiating an
arbitrary OAuth 2 client credentials grant flow.

#### Options

| Name | Description | Default | Required |
|------|-------------|---------|----------|
| `discovery_url` | The URL at which to discover authorization code and token URLs. | None | Either this or the other two urls |
| `token_url` | The URL to use for exchanging temporary codes and refreshing access tokens. | None | Either this or `discovery_url` |
| `auth_style` | How to authenticate to the token URL. If specified, must be one of `in_header` or `in_params`. | Automatically detect | No |
