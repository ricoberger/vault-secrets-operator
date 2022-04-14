package vault

import (
	"fmt"
	"github.com/go-logr/logr"
	"time"
)

func (c *Client) GetDatabaseCreds(path string, role string, mylog logr.Logger) (map[string][]byte, *time.Time, error) {
	r, err := c.client.Logical().ReadWithData(path+"/creds/"+role, map[string][]string{})
	if err != nil {
		return nil, nil, err
	}

	if r == nil {
		return nil, nil, fmt.Errorf("database credentials is nil")
	}

	data, err := convertData(r.Data, []string{"username", "password"}, false)
	if err != nil {
		return nil, nil, err
	}

	expiration := time.Now().Add(time.Duration(r.LeaseDuration) * time.Second)

	return data, &expiration, nil
}
