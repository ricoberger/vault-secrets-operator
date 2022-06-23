package vault

import (
	"fmt"
	"github.com/hashicorp/vault/api"
	"time"
)

func (c *Client) GetDatabaseCreds(path string, role string) (*api.Secret, *time.Time, error) {
	secret, err := c.client.Logical().ReadWithData(path+"/creds/"+role, map[string][]string{})
	if err != nil {
		return nil, nil, err
	}

	if secret == nil {
		return nil, nil, fmt.Errorf("database credentials is nil")
	}

	expiresAt := time.Now().Add(time.Duration(secret.LeaseDuration) * time.Second)

	return secret, &expiresAt, nil
}

func (c *Client) DatabaseRenderData(secret *api.Secret) (map[string][]byte, error) {
	return convertData(secret.Data, []string{"username", "password"}, false)
}
