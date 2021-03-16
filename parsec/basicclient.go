// Copyright 2021 Contributors to the Parsec project.
// SPDX-License-Identifier: Apache-2.0

package parsec

import (
	"fmt"
	"reflect"

	"github.com/parallaxsecond/parsec-client-go/interface/auth"
	"github.com/parallaxsecond/parsec-client-go/interface/operations"
	"github.com/parallaxsecond/parsec-client-go/interface/requests"
	"github.com/parallaxsecond/parsec-client-go/parsec/algorithm"
)

// BasicClient is a Parsec client representing a connection and set of API implementations
type BasicClient struct {
	opclient         *operations.Client
	auth             Authenticator
	implicitProvider ProviderID
	config           *ClientConfig
}

// InitClient initializes a Parsec client
// This will autoselect the first provider returned by the parsec service.  It will also attempt to
// select the first available authenticator it can configure.  The config can either be a *ClientConfig or a string.
// If it is a string, then this is used as an application name for a default Direct Authenticator.
// If nil is passed, then the client will try and find the first supported authenticator that requires no configuration.
func InitClient(config interface{}) (*BasicClient, error) {
	var clientConfig *ClientConfig
	if config == nil {
		clientConfig = NewClientConfig()
	} else {
		switch confSpecific := config.(type) {
		case string:
			clientConfig = DirectAuthConfigData(confSpecific)
		case *ClientConfig:
			clientConfig = confSpecific
		default:
			return nil, fmt.Errorf("could not create configuration from type %v", reflect.TypeOf(config))
		}
	}

	var opclient *operations.Client
	var err error
	if clientConfig.connection == nil {
		opclient, err = operations.InitClient()
	} else {
		opclient, err = operations.InitClientFromConnection(clientConfig.connection)
	}
	if err != nil {
		return nil, err
	}

	bc := BasicClient{
		opclient:         opclient,
		implicitProvider: ProviderCore,
		auth:             NewNoAuthAuthenticator(),
		config:           clientConfig,
	}

	err = bc.selectDefaultProvider()
	if err != nil {
		return nil, err
	}
	err = bc.selectDefaultAuthenticator()
	if err != nil {
		return nil, err
	}

	return &bc, nil
}

// Close the client and any underlying connections
func (c *BasicClient) Close() error {
	return c.opclient.Close()
}

// SetImplicitProvider sets the provider to use for non-core operations
func (c *BasicClient) SetImplicitProvider(provider ProviderID) {
	c.implicitProvider = provider
}

// GetImplicitProvider returns the provider used for non-core operations
func (c *BasicClient) GetImplicitProvider() ProviderID {
	return c.implicitProvider
}

func (c *BasicClient) selectDefaultProvider() error {
	c.implicitProvider = ProviderCore // We know this one is always present.
	availableProviders, err := c.ListProviders()
	if err != nil {
		return err
	}
	if len(availableProviders) > 0 {
		c.implicitProvider = availableProviders[0].ID
	}
	return nil
}

func (c *BasicClient) selectDefaultAuthenticator() error {
	availableAuthenticators, err := c.ListAuthenticators()
	if err != nil {
		return err
	}
Loop:
	for _, authenticator := range availableAuthenticators {
		switch authenticator.ID { //nolint:exhaustive // we cover everything with the default
		case AuthDirect:
			// See if we have data for this authenticator type
			if data, ok := c.config.authenticatorData[auth.AuthDirect]; ok {
				if appName, ok := data.(string); ok {
					c.auth = NewDirectAuthenticator(appName)
					break Loop
				} else {
					panic("Direct authenticator data is of wrong type.") // this should not happen
				}
			} // no data for this authenticator, carry on trying
		case AuthUnixPeerCredentials:
			c.auth = NewUnixPeerAuthenticator()
			break Loop
		default:
			continue
		}
	}
	return nil
}

func (c *BasicClient) GetAuthenticatorType() AuthenticatorType {
	return c.auth.GetAuthenticatorType()
}

// Ping server and return wire protocol major and minor version number
func (c BasicClient) Ping() (uint8, uint8, error) { //nolint:gocritic
	return c.opclient.Ping(requests.ProviderCore, c.auth.toNativeAuthenticator())
}

// ListProviders returns a list of the providers supported by the server.
func (c BasicClient) ListProviders() ([]*ProviderInfo, error) {
	nativeProv, err := c.opclient.ListProviders(requests.ProviderCore, c.auth.toNativeAuthenticator())
	if err != nil {
		return nil, err
	}
	providers := make([]*ProviderInfo, len(nativeProv))
	for i, p := range nativeProv {
		providers[i] = newProviderInfoFromOp(p)
	}
	return providers, nil
}

// ListOpcodes list the opcodes for a provider
func (c BasicClient) ListOpcodes(providerID ProviderID) ([]uint32, error) {
	return c.opclient.ListOpcodes(requests.ProviderCore, c.auth.toNativeAuthenticator(), uint32(providerID))
}

// ListKeys obtain keys stored for current application
func (c BasicClient) ListKeys() ([]*KeyInfo, error) {
	retkeys, err := c.opclient.ListKeys(requests.ProviderCore, c.auth.toNativeAuthenticator())
	if err != nil {
		return nil, err
	}

	keys := make([]*KeyInfo, len(retkeys))
	for idx, key := range retkeys {
		keys[idx], err = newKeyInfoFromOp(key)
		if err != nil {
			return nil, err
		}
	}
	return keys, nil
}

// ListAuthenticators obtain authenticators supported by server
func (c BasicClient) ListAuthenticators() ([]*AuthenticatorInfo, error) {
	retauths, err := c.opclient.ListAuthenticators(requests.ProviderCore, c.auth.toNativeAuthenticator())
	if err != nil {
		return nil, err
	}
	auths := make([]*AuthenticatorInfo, len(retauths))
	for idx, auth := range retauths {
		a, err := newAuthenticatorInfoFromOp(auth)
		if err != nil {
			return nil, err
		}
		auths[idx] = a
	}
	return auths, nil
}

// PsaGenerateKey create key named name with attributes
func (c BasicClient) PsaGenerateKey(name string, attributes *KeyAttributes) error {
	if !c.implicitProvider.HasCrypto() {
		return fmt.Errorf("provider does not support crypto operation")
	}

	ka, err := attributes.toWireInterface()

	if err != nil {
		return err
	}
	fmt.Printf("keyattributes: %+v\n", ka)
	return c.opclient.PsaGenerateKey(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), name, ka)
}

// PsaDestroyKey destroys a key with given name
func (c BasicClient) PsaDestroyKey(name string) error {
	if !c.implicitProvider.HasCrypto() {
		return fmt.Errorf("provider does not support crypto operation")
	}
	return c.opclient.PsaDestroyKey(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), name)
}

// PsaHashCompute calculates a hash of a message using specified algorithm
func (c BasicClient) PsaHashCompute(message []byte, alg algorithm.HashAlgorithmType) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	return c.opclient.PsaHashCompute(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), message, hashAlgToWire(alg))
}

// PsaSignMessage signs message using signingKey and algorithm, returning the signature.
func (c BasicClient) PsaSignMessage(signingKey string, message []byte, alg *algorithm.AsymmetricSignatureAlgorithm) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algAsymmetricSigToWire(alg)
	if err != nil {
		return nil, err
	}
	return c.opclient.PsaSignMessage(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), signingKey, message, opalg)
}

// PsaSignHash signs hash using signingKey and algorithm, returning the signature.
func (c BasicClient) PsaSignHash(signingKey string, hash []byte, alg *algorithm.AsymmetricSignatureAlgorithm) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algAsymmetricSigToWire(alg)
	if err != nil {
		return nil, err
	}
	return c.opclient.PsaSignHash(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), signingKey, hash, opalg)
}

// PsaVerifyMessage verify a signature  of message with verifyingKey using signature algorithm alg.
func (c BasicClient) PsaVerifyMessage(verifyingKey string, message, signature []byte, alg *algorithm.AsymmetricSignatureAlgorithm) error {
	if !c.implicitProvider.HasCrypto() {
		return fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algAsymmetricSigToWire(alg)
	if err != nil {
		return err
	}
	return c.opclient.PsaVerifyMessage(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), verifyingKey, message, signature, opalg)
}

// PsaVerifyHash verify a signature  of hash with verifyingKey using signature algorithm alg.
func (c BasicClient) PsaVerifyHash(verifyingKey string, hash, signature []byte, alg *algorithm.AsymmetricSignatureAlgorithm) error {
	if !c.implicitProvider.HasCrypto() {
		return fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algAsymmetricSigToWire(alg)
	if err != nil {
		return err
	}
	return c.opclient.PsaVerifyHash(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), verifyingKey, hash, signature, opalg)
}

// PsaCipherEncrypt carries out symmetric encryption on plaintext using defined key/algorithm, returning ciphertext
func (c BasicClient) PsaCipherEncrypt(keyName string, alg *algorithm.Cipher, plaintext []byte) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algCipherAlgToWire(alg)
	if err != nil {
		return nil, err
	}
	return c.opclient.PsaCipherEncrypt(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName, opalg, plaintext)
}

// PsaCipherDecrypt decrypts symmetrically encrypted ciphertext using defined key/algorithm, returning plaintext
func (c BasicClient) PsaCipherDecrypt(keyName string, alg *algorithm.Cipher, ciphertext []byte) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algCipherAlgToWire(alg)
	if err != nil {
		return nil, err
	}
	return c.opclient.PsaCipherDecrypt(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName, opalg, ciphertext)
}

func (c BasicClient) PsaAeadDecrypt(keyName string, alg *algorithm.AeadAlgorithm, nonce, additionalData, ciphertext []byte) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algAeadAlgToWire(alg)
	if err != nil {
		return nil, err
	}
	return c.opclient.PsaAeadDecrypt(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName, opalg, nonce, additionalData, ciphertext)
}

func (c BasicClient) PsaAeadEncrypt(keyName string, alg *algorithm.AeadAlgorithm, nonce, additionalData, plaintext []byte) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algAeadAlgToWire(alg)
	if err != nil {
		return nil, err
	}
	return c.opclient.PsaAeadEncrypt(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName, opalg, nonce, additionalData, plaintext)
}

func (c BasicClient) PsaExportKey(keyName string) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	return c.opclient.PsaExportKey(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName)
}

func (c BasicClient) PsaImportKey(keyName string, attributes *KeyAttributes, data []byte) error {
	if !c.implicitProvider.HasCrypto() {
		return fmt.Errorf("provider does not support crypto operation")
	}
	opattrs, err := attributes.toWireInterface()
	if err != nil {
		return err
	}
	return c.opclient.PsaImportKey(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName, opattrs, data)
}

func (c BasicClient) PsaExportPublicKey(keyName string) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	return c.opclient.PsaExportPublicKey(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName)
}

func (c BasicClient) PsaGenerateRandom(size uint64) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	return c.opclient.PsaGenerateRandom(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), size)
}

func (c BasicClient) PsaMACCompute(keyName string, alg *algorithm.MacAlgorithm, input []byte) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algMacAlgToWire(alg)
	if err != nil {
		return nil, err
	}
	return c.opclient.PsaMACCompute(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName, opalg, input)
}

func (c BasicClient) PsaMACVerify(keyName string, alg *algorithm.MacAlgorithm, input, mac []byte) error {
	if !c.implicitProvider.HasCrypto() {
		return fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algMacAlgToWire(alg)
	if err != nil {
		return err
	}
	return c.opclient.PsaMACVerify(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName, opalg, input, mac)
}

func (c BasicClient) PsaRawKeyAgreement(alg *algorithm.KeyAgreementRaw, privateKey string, peerKey []byte) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algKeyAgreementRawAlgToWire(alg)
	if err != nil {
		return nil, err
	}
	return c.opclient.PsaRawKeyAgreement(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), opalg.GetRaw().Enum(), privateKey, peerKey)
}

func (c BasicClient) PsaAsymmetricDecrypt(keyName string, alg *algorithm.AsymmetricEncryptionAlgorithm, salt, ciphertext []byte) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algAsymmetricEncryptionAlgToWire(alg)
	if err != nil {
		return nil, err
	}
	return c.opclient.PsaAsymmetricDecrypt(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName, opalg, salt, ciphertext)
}

func (c BasicClient) PsaAsymmetricEncrypt(keyName string, alg *algorithm.AsymmetricEncryptionAlgorithm, salt, plaintext []byte) ([]byte, error) {
	if !c.implicitProvider.HasCrypto() {
		return nil, fmt.Errorf("provider does not support crypto operation")
	}
	opalg, err := algAsymmetricEncryptionAlgToWire(alg)
	if err != nil {
		return nil, err
	}
	return c.opclient.PsaAsymmetricEncrypt(requests.ProviderID(c.implicitProvider), c.auth.toNativeAuthenticator(), keyName, opalg, salt, plaintext)
}
