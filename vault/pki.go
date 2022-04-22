package vault

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/vault/api"
	"time"
)

func (c *Client) GetCertificate(path string, role string, options map[string]string) (*api.Secret, *time.Time, error) {
	optionsI := make(map[string]interface{}, len(options))
	for k, v := range options {
		optionsI[k] = v
	}

	secret, err := c.client.Logical().Write(path+"/issue/"+role, optionsI)
	if err != nil {
		return nil, nil, err
	}

	if secret == nil {
		return nil, nil, fmt.Errorf("certificate is nil")
	}

	exp, err := secret.Data["expiration"].(json.Number).Int64()
	if err != nil {
		return nil, nil, fmt.Errorf("error converting expiration date: %v", err)
	}
	expiresAt := time.Unix(exp, 0)

	return secret, &expiresAt, nil
}

func (c *Client) PKIConvertData(secret *api.Secret) (map[string][]byte, error) {
	return convertData(secret.Data, []string{
		"certificate",
		"expiration",
		"issuing_ca",
		"private_key",
		"private_key_type",
		"serial_number"}, false)
}
