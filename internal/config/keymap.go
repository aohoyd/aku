package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Keymap holds key binding configuration, loaded from keymap.yaml.
type Keymap struct {
	Bindings   []Binding `yaml:"bindings,omitempty"`
	bindingSet *BindingSet
}

// DefaultKeymap returns a Keymap with default bindings.
func DefaultKeymap() *Keymap {
	return &Keymap{Bindings: DefaultBindings()}
}

// LoadKeymap loads key bindings from a YAML file.
// Missing file returns defaults. User bindings are appended after defaults.
func LoadKeymap(path string) (*Keymap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultKeymap(), nil
		}
		return nil, err
	}

	km := &Keymap{}
	if err := yaml.Unmarshal(data, km); err != nil {
		return nil, err
	}

	if len(km.Bindings) > 0 {
		km.Bindings = append(DefaultBindings(), km.Bindings...)
	} else {
		km.Bindings = DefaultBindings()
	}

	return km, nil
}

// BindingSet returns the compiled BindingSet, building it on first access.
func (k *Keymap) BindingSet() *BindingSet {
	if k.bindingSet == nil {
		if len(k.Bindings) > 0 {
			k.bindingSet = NewBindingSet(k.Bindings)
		} else {
			k.bindingSet = NewBindingSet(DefaultBindings())
		}
	}
	return k.bindingSet
}
