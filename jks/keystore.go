package jks

import (
	"bytes"
	"crypto/x509"
	"math/rand"
	"os"

	"encoding/pem"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	keystore "github.com/pavel-v-chernykh/keystore-go/v3"
	"github.com/ricoberger/vault-secrets-operator/vault"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	// log is our customized logger.
	log = logf.Log.WithName("jks")
)

// GetKeystoreFromSecret returns jks (keystore or truststore) with password, from vault secret keys.
func GetKeystoreFromSecret(secret *corev1.Secret, data map[string][]byte, KeystoreK8sSecretKeyName string, KeystorePassK8sSecretKeyName string, KeystoreType string, Client *vault.Client) ([]byte, string, error) {

	if !(KeystoreType == "keystore" || KeystoreType == "truststore") {
		return []byte{}, "", fmt.Errorf("Jks type must either be 'keystore' or 'truststore', recieved %s", KeystoreType)
	}
	var keyStorePass string

	//check if there's already a keystore pass in k8s secret.
	existingKeystorePass, exists := secret.Data[KeystorePassK8sSecretKeyName]
	if exists {
		log.Info("keystore pass already exists in k8s secret, using that...")
		keyStorePass = string(existingKeystorePass)
	} else {
		log.Info("generating random keystore pass")
		keyStorePass = genPass()
	}

	// create new keystore/truststore to later compare with existing, if exists that is.
	var newKeystore keystore.KeyStore
	var err error
	if KeystoreType == "keystore" {
		newKeystore, err = createKeystore(data, keyStorePass)
		if err != nil {
			log.Error(err, "can't create new keystore", "keystore", newKeystore)
			return []byte{}, "", err
		}
	} else {
		newKeystore, err = createTruststore(data, keyStorePass, Client)
		if err != nil {
			log.Error(err, "can't create new keystore", "keystore", newKeystore)
			return []byte{}, "", err
		}
	}

	//check if there's already a keystore/truststore in k8s secret.
	existingKeystoreEncoded, exists := secret.Data[KeystoreK8sSecretKeyName]
	if exists {
		log.Info("keystore already exists in k8s secret, checking if up to date...")

		existingKeystore := readKeystore(existingKeystoreEncoded, keyStorePass)

		same, err := compareKeystores(existingKeystore, newKeystore, KeystoreType)
		if err != nil {
			log.Error(err, "error comparing keystores")
			return []byte{}, "", err
		} else if same {
			log.Info("no changes will be made to keystore in k8s secret, all seems good & reconciled")
			buffer := bytes.Buffer{}
			buffer.Write(existingKeystoreEncoded)
			return buffer.Bytes(), keyStorePass, nil
		} else {
			log.Info("reconciling keystore in k8s secret, looks out of date.")
			buffer := bytes.Buffer{}
			err := keystore.Encode(&buffer, newKeystore, []byte(keyStorePass))
			if err != nil {
				log.Error(err, "unable to encode keystore", "keystore", newKeystore)
				return []byte{}, "", err
			}
			return buffer.Bytes(), keyStorePass, nil
		}
	} else {
		log.Info("creating new keystore from specified keys in vault.")
		buffer := bytes.Buffer{}
		err := keystore.Encode(&buffer, newKeystore, []byte(keyStorePass))
		if err != nil {
			log.Error(err, "unable to encode keystore", "keystore", newKeystore)
			return []byte{}, "", err
		}
		return buffer.Bytes(), keyStorePass, nil
	}
}

func compareKeystores(ks1 keystore.KeyStore, ks2 keystore.KeyStore, ksType string) (bool, error) {
	// straightout reflect.DeepEqual(ks1, ks2) always gets back false, since there is a CreationDate key in &keystore.PrivateKeyEntry that will always be different.

	if !(ksType == "keystore" || ksType == "truststore") {
		return false, fmt.Errorf("Jks type must either be 'keystore' or 'truststore', recieved %s", ksType)
	}

	// get all entries/aliases of ks1 into a slice
	ks1Aliases := make([]string, 0, len(ks1))
	for k := range ks1 {
		ks1Aliases = append(ks1Aliases, k)
	}
	sort.Strings(ks1Aliases)

	// get all entries/aliases of ks2 into a slice
	ks2Aliases := make([]string, 0, len(ks2))
	for k := range ks2 {
		ks2Aliases = append(ks2Aliases, k)
	}
	sort.Strings(ks2Aliases)

	// make sure both keystores have same alias keys
	if !reflect.DeepEqual(ks1Aliases, ks2Aliases) {
		return false, nil
	}

	// then iterate over and dig into the content (priv keys & cert chains) for each entry
	for key, value := range ks1 {

		if ksType == "keystore" {
			privKey1 := value.(*keystore.PrivateKeyEntry).PrivateKey
			certChain1 := value.(*keystore.PrivateKeyEntry).CertificateChain

			privKey2 := ks2[key].(*keystore.PrivateKeyEntry).PrivateKey
			certChain2 := ks2[key].(*keystore.PrivateKeyEntry).CertificateChain

			if !reflect.DeepEqual(privKey1, privKey2) {
				return false, nil
			}
			if !reflect.DeepEqual(certChain1, certChain2) {
				return false, nil
			}
		} else {
			cert1 := value.(*keystore.TrustedCertificateEntry).Certificate
			cert2 := ks2[key].(*keystore.TrustedCertificateEntry).Certificate

			if !reflect.DeepEqual(cert1, cert2) {
				return false, nil
			}
		}

	}

	// looks like ks1 & ks2 are the same after all!
	return true, nil
}

func readKeystore(encodedKeystore []byte, password string) keystore.KeyStore {
	buffer := bytes.Buffer{}
	buffer.Write(encodedKeystore)
	keyStore, err := keystore.Decode(&buffer, []byte(password))
	if err != nil {
		log.Error(err, "error decode keystore in k8s secret")
	}
	return keyStore
}

func createKeystore(data map[string][]byte, password string) (keystore.KeyStore, error) {

	keyStore := keystore.KeyStore{}

	for key, value := range data {

		block, rest := pem.Decode([]byte(value))

		if block == nil {
			return nil, fmt.Errorf("no block found in key %s entry, private key should have at least one pem block", key)
		}
		if !strings.Contains(block.Type, "RSA PRIVATE KEY") {
			return nil, fmt.Errorf("private key block of %s is not of type RSA PRIVATE KEY", key)
		}

		rawRsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("could not parse/decode private key of %s's PEM block to PKCS #1", key)
		}

		pkcs8EncodedPrKey, err := x509.MarshalPKCS8PrivateKey(rawRsaKey)
		if err != nil {
			return nil, fmt.Errorf("could not convert/encode private key of %s's PEM block to PKCS #8", key)
		}

		certs := []keystore.Certificate{}
		for block, rest := pem.Decode(rest); block != nil; block, rest = pem.Decode(rest) {
			certs = append(certs, keystore.Certificate{
				Type:    "X.509",
				Content: block.Bytes,
			})
		}

		keyStore[key] = &keystore.PrivateKeyEntry{
			Entry: keystore.Entry{
				CreationTime: time.Now(),
			},
			PrivateKey:       pkcs8EncodedPrKey,
			CertificateChain: certs,
		}
	}
	return keyStore, nil
}

func createTruststore(data map[string][]byte, password string, client *vault.Client) (keystore.KeyStore, error) {

	trustStore, err := createBaseTruststore(client)
	if err != nil {
		return nil, errors.New("can't create base Truststore")
	}

	for key, value := range data {

		for block, rest := pem.Decode(value); block != nil; block, rest = pem.Decode(rest) {

			if strings.Contains(block.Type, "KEY") {
				return nil, errors.New("found Key in what's supposed to be a Cert")
			}

			trustStore[key] = &keystore.TrustedCertificateEntry{
				Entry: keystore.Entry{
					CreationTime: time.Now(),
				},
				Certificate: keystore.Certificate{
					Type:    "X.509",
					Content: block.Bytes,
				},
			}
		}
	}

	return trustStore, nil
}

func createBaseTruststore(client *vault.Client) (keystore.KeyStore, error) {

	jksBaseTruststoreLocation := "/cacerts"
	jksBaseTruststorePass := []byte("changeit")
	vaultCaCertPaths := strings.Split(os.Getenv("VAULT_CA_CERT_PATHS"), ",")

	baseTruststore, err := readKeystoreFromFile(jksBaseTruststoreLocation, jksBaseTruststorePass)
	if err != nil {
		log.Error(err, "can't read base Truststore at ", jksBaseTruststoreLocation)
		return nil, err
	}

	// add all pki ca certs required
	for _, vaultCaCertPath := range vaultCaCertPaths {

		caCert, err := client.GetCaCert(vaultCaCertPath)
		if err != nil {
			log.Error(err, "can't read base vault Ca Cert at ", vaultCaCertPath)
			return nil, err
		}

		baseTruststore["pz_vault:"+vaultCaCertPath] = &keystore.TrustedCertificateEntry{
			Entry: keystore.Entry{
				CreationTime: time.Now(),
			},
			Certificate: *caCert,
		}
	}

	return baseTruststore, nil

}

func readKeystoreFromFile(filename string, password []byte) (keystore.KeyStore, error) {

	f, err := os.Open(filename)
	if err != nil {
		log.Error(err, "can't os.Open("+filename+") for base Truststore")
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Error(err, "can't f.Close() after reading base Truststore")
		}
	}()
	keyStore, err := keystore.Decode(f, password)
	if err != nil {
		log.Error(err, "can't decode base Truststore")
		return nil, err
	}
	return keyStore, nil

}

func genPass() string {

	rand.Seed(time.Now().UnixNano())
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789")
	length := 8
	buf := make([]rune, length)
	for i := range buf {
		buf[i] = chars[rand.Intn(len(chars))]
	}
	str := string(buf)
	return str
}
