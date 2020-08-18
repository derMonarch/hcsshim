package clone

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/regstate"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

const (
	configRoot = "LateClone"
	configKey  = "UVMConfig"
)

// When encoding interfaces gob requires us to register the struct types that we will be
// using under those interfaces. This registration needs to happen on both sides i.e the
// side which encodes the data (i.e the shim process of the template) and the side which
// decodes the data (i.e the shim process of the clone).
// Go init function: https://golang.org/doc/effective_go.html#init
func init() {
	// Register the pointer to structs because that is what is being stored.
	gob.Register(&uvm.VSMBShare{})
	gob.Register(&uvm.SCSIMount{})
}

func encodeTemplateConfig(utc *uvm.UVMTemplateConfig) ([]byte, error) {
	var buf bytes.Buffer

	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(utc); err != nil {
		return nil, fmt.Errorf("error while encoding template config: %s", err)
	}
	return buf.Bytes(), nil
}

func decodeTemplateConfig(encodedBytes []byte) (*uvm.UVMTemplateConfig, error) {
	var utc uvm.UVMTemplateConfig

	reader := bytes.NewReader(encodedBytes)
	decoder := gob.NewDecoder(reader)
	if err := decoder.Decode(&utc); err != nil {
		return nil, fmt.Errorf("error while decoding template config: %s", err)
	}
	return &utc, nil
}

// loadPersistedUVMConfig loads a persisted config from the registry that matches the given ID
// If not found returns `regstate.NotFoundError`
func loadPersistedUVMConfig(id string) ([]byte, error) {
	sk, err := regstate.Open(configRoot, false)
	if err != nil {
		return nil, err
	}
	defer sk.Close()

	var encodedConfig []byte
	if err := sk.Get(id, configKey, &encodedConfig); err != nil {
		return nil, err
	}
	return encodedConfig, nil
}

// storePersistedUVMConfig stores the given config to the registry.
// If the store fails returns the store error.
func storePersistedUVMConfig(id string, encodedConfig []byte) error {
	sk, err := regstate.Open(configRoot, false)
	if err != nil {
		return err
	}
	defer sk.Close()

	if err := sk.Create(id, configKey, encodedConfig); err != nil {
		return err
	}
	return nil
}

// removePersistedUVMConfig removes any persisted state associated with this config. If the config
// is not found in the registery `Remove` returns no error.
func removePersistedUVMConfig(id string) error {
	sk, err := regstate.Open(configRoot, false)
	if err != nil {
		if regstate.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	defer sk.Close()

	if err := sk.Remove(id); err != nil {
		if regstate.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	return nil
}

// Saves all the information required to create a clone from the template
// of this container into the registry.
func SaveTemplateConfig(ctx context.Context, utc *uvm.UVMTemplateConfig) error {
	_, err := loadPersistedUVMConfig(utc.UVMID)
	if !regstate.IsNotFoundError(err) {
		return fmt.Errorf("parent VM(ID: %s) config shouldn't exit in registry (%s)", utc.UVMID, err)
	}

	encodedBytes, err := encodeTemplateConfig(utc)
	if err != nil {
		return fmt.Errorf("failed to encode template config: %s", err)
	}

	if err := storePersistedUVMConfig(utc.UVMID, encodedBytes); err != nil {
		return fmt.Errorf("failed to store encoded template config: %s", err)
	}

	return nil
}

// Removes all the state associated with the template with given ID
// If there is no state associated with this ID then the function simply returns without
// doing anything.
func RemoveSavedTemplateConfig(id string) error {
	return removePersistedUVMConfig(id)
}

// Retrieves the UVMTemplateConfig for the template with given ID from the registry.
func FetchTemplateConfig(ctx context.Context, id string) (*uvm.UVMTemplateConfig, error) {
	encodedBytes, err := loadPersistedUVMConfig(id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch encoded template config: %s", err)
	}

	utc, err := decodeTemplateConfig(encodedBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode template config: %s", err)
	}
	return utc, nil
}
