package vault

import (
	"fmt"
)

func (c *Client) GetCertificate(path string, role string, options map[string]string) (map[string][]byte, error) {
	log.Info(fmt.Sprintf("Get certificate %s", ""))

	optionsI := make(map[string]interface{}, len(options))
	for k, v := range options {
		optionsI[k] = v
	}

	r, err := c.client.Logical().Write(path+"/issue/"+role, optionsI)
	if err != nil {
		return nil, err
	}

	if r == nil {
		return nil, fmt.Errorf("certificate is nil")
	}

	data := map[string][]byte{
		"certificate":      []byte(r.Data["certificate"].(string)),
		"issuing_ca":       []byte(r.Data["issuing_ca"].(string)),
		"private_key":      []byte(r.Data["private_key"].(string)),
		"private_key_type": []byte(r.Data["private_key_type"].(string)),
		"serial_number":    []byte(r.Data["serial_number"].(string)),
	}

	return data, nil
}
