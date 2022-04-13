package vault

import (
	"encoding/json"
	"fmt"
	"time"
)

func convertData(data map[string]interface{}) map[string][]byte {
	return map[string][]byte{
		"certificate":      []byte(data["certificate"].(string)),
		"expiration":       []byte(data["expiration"].(json.Number).String()),
		"issuing_ca":       []byte(data["issuing_ca"].(string)),
		"private_key":      []byte(data["private_key"].(string)),
		"private_key_type": []byte(data["private_key_type"].(string)),
		"serial_number":    []byte(data["serial_number"].(string)),
	}
}

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

	return convertData(r.Data), &expiration, nil
}
