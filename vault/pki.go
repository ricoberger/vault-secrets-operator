package vault

import (
	"encoding/json"
	"fmt"
	"time"
)

func (c *Client) GetCertificate(path string, role string, options map[string]string) (map[string][]byte, *time.Time, error) {
	optionsI := make(map[string]interface{}, len(options))
	for k, v := range options {
		optionsI[k] = v
	}

	r, err := c.client.Logical().Write(path+"/issue/"+role, optionsI)
	if err != nil {
		return nil, nil, err
	}

	if r == nil {
		return nil, nil, fmt.Errorf("certificate is nil")
	}

	exp, err := r.Data["expiration"].(json.Number).Int64()
	if err != nil {
		return nil, nil, fmt.Errorf("error converting expiration date: %v", err)
	}
	expiration := time.Unix(exp, 0)

	data, err := convertData(r.Data, []string{
		"certificate",
		"expiration",
		"issuing_ca",
		"private_key",
		"private_key_type",
		"serial_number"}, false)
	if err != nil {
		return nil, nil, err
	}

	return data, &expiration, nil
}
