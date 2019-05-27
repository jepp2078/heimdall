package main

type Configuration struct {
	ConfigVersion string                `yaml:"configVersion"`
	Metadata      Metadata              `yaml:"metadata"`
	Entities      []ConfigurationEntity `yaml:"configuration"`
}

type Metadata struct {
	Author    string `yaml:"author"`
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

type ConfigurationEntity struct {
	Name      string `yaml:"name"`
	Value     string `yaml:"value"`
	Encrypted bool   `yaml:"encrypted"`
}
