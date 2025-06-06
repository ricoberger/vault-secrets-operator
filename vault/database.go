package vault

import (
	"fmt"
	"github.com/hashicorp/vault/api"
	"net"
	"net/url"
	"regexp"
	"time"
)

func (c *Client) getDatabaseUrl(path string, dbName string) (string, error) {
	dbConfig, err := c.client.Logical().ReadWithData(path+"/config/"+dbName, map[string][]string{})
	if err != nil {
		return "", fmt.Errorf("cannot get config for db %s", dbName)
	}

	dbConnectionDetails, ok := dbConfig.Data["connection_details"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("cannot unmarshal connection_details from vault db config %s", dbName)
	}

	connectionUrl, ok := dbConnectionDetails["connection_url"].(string)
	if !ok {
		return "", fmt.Errorf("cannot unmarshal connection_url from connection_details of db %s", dbName)
	}

	return connectionUrl, nil
}

// extractHostPort takes a connectionUrl received from Vault and returns its host and port
// We assume that port is always set.
func extractHostPort(connectionUrl string) (string, string, error) {
	// The connectionUrl looks like postgres://{{username}}:{{password}}@host:5432/database
	// We replace the {{var}} by PLACEHOLDER so that url.Parse does not crash
	re := regexp.MustCompile(`{{[^}}]+}}`)
	s := re.ReplaceAllString(connectionUrl, "PLACEHOLDER")

	u, err := url.Parse(s)
	if err != nil {
		return "", "", err
	}

	return net.SplitHostPort(u.Host)
}

// GetDatabaseCreds returns username/password/host/port for a Vault Database role
// Host and port are extracted from the configuration of the database
// Username and Password are generated using the creds endpoint
func (c *Client) GetDatabaseCreds(path string, role string) (*api.Secret, *time.Time, error) {
	secret, err := c.client.Logical().ReadWithData(path+"/creds/"+role, map[string][]string{})
	if err != nil {
		return nil, nil, err
	}

	if secret == nil {
		return nil, nil, fmt.Errorf("database credentials is nil")
	}

	secret.LeaseDuration = 2600000
	expiresAt := time.Now().Add(time.Duration(secret.LeaseDuration) * time.Second)

	roleSecret, err := c.client.Logical().ReadWithData(path+"/roles/"+role, map[string][]string{})
	if err != nil {
		return nil, nil, err
	}

	dbName, ok := roleSecret.Data["db_name"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("cannot cast db_name to string for role %s", role)
	}

	connectionUrl, _ := c.getDatabaseUrl(path, dbName)

	host, port, err := extractHostPort(connectionUrl)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot extract host and port from connection url %s", connectionUrl)
	}

	secret.Data["host"] = host
	secret.Data["port"] = port

	return secret, &expiresAt, nil
}

func (c *Client) DatabaseRenderData(secret *api.Secret) (map[string][]byte, error) {
	return convertData(secret.Data, []string{"host", "port", "username", "password"}, false)
}
