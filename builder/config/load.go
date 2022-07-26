package config

import (
	"github.com/spf13/viper"

	"github.com/uor-framework/uor-client-go/builder/api/v1alpha1"
)

func ReadConfig(configName string) (v1alpha1.DataSetConfiguration, error) {
	var configuration v1alpha1.DataSetConfiguration

	viper.SetConfigName(configName)
	viper.AddConfigPath(".")
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err != nil {
		return configuration, err
	}

	err = viper.Unmarshal(&configuration)
	if err != nil {
		return configuration, err
	}

	return configuration, err
}
