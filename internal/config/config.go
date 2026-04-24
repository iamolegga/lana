package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/iamolegga/goenvsubst"
	"go.yaml.in/yaml/v3"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
	validate.RegisterStructValidation(validateOAuthProvider, OAuthProvider{})
}

func validateOAuthProvider(sl validator.StructLevel) {
	p := sl.Current().Interface().(OAuthProvider)

	hasAppleFields := p.TeamID != "" || p.KeyID != "" || p.PrivateKeyFile != "" || p.ServicesID != ""

	if hasAppleFields {
		if p.ServicesID == "" {
			sl.ReportError(p.ServicesID, "ServicesID", "ServicesID", "required_with_apple", "")
		}
		if p.TeamID == "" {
			sl.ReportError(p.TeamID, "TeamID", "TeamID", "required_with_apple", "")
		}
		if p.KeyID == "" {
			sl.ReportError(p.KeyID, "KeyID", "KeyID", "required_with_apple", "")
		}
		if p.PrivateKeyFile == "" {
			sl.ReportError(p.PrivateKeyFile, "PrivateKeyFile", "PrivateKeyFile", "required_with_apple", "")
		} else if _, err := os.Stat(p.PrivateKeyFile); err != nil {
			sl.ReportError(p.PrivateKeyFile, "PrivateKeyFile", "PrivateKeyFile", "file", "")
		}
	} else {
		if p.ClientID == "" {
			sl.ReportError(p.ClientID, "ClientID", "ClientID", "required", "")
		}
		if p.ClientSecret == "" {
			sl.ReportError(p.ClientSecret, "ClientSecret", "ClientSecret", "required", "")
		}
	}
}

type Config struct {
	Env    string `yaml:"env"       validate:"oneof=development production"`
	Server struct {
		Port string `yaml:"port" validate:"gt=0"`
	} `yaml:"server"`
	Cookie struct {
		Secret string `yaml:"secret" validate:"required"`
		Name   string `yaml:"name"`
	} `yaml:"cookie"`
	RateLimit struct {
		RequestsPerMinute  int           `yaml:"requests_per_minute" validate:"gt=0"`
		CleanupInterval    time.Duration `yaml:"cleanup_interval"`
		XForwardedForIndex int           `yaml:"x_forwarded_for_index"`
	} `yaml:"ratelimit"`
	Logging struct {
		Level  string `yaml:"level" validate:"oneof=debug info warn error"`
		Format string `yaml:"format" validate:"oneof=text json"`
	} `yaml:"logging"`
	Metrics struct {
		Enable    bool   `yaml:"enable"`
		GoMetrics bool   `yaml:"go_metrics"`
		Path      string `yaml:"path"`
	} `yaml:"metrics"`
	Admin struct {
		Enabled bool `yaml:"enabled"`
		Port    int  `yaml:"port" validate:"required_if=Enabled true,omitempty,min=1,max=65535"`
	} `yaml:"admin"`
	Hosts map[string]HostConfig `yaml:"hosts"     validate:"required,dive,keys,required,endkeys,required"`
}

type HostConfig struct {
	LoginDir            string                   `yaml:"login_dir" validate:"required"`
	AllowedRedirectURLs []string                 `yaml:"allowed_redirect_urls" validate:"required,min=1,dive,required"`
	Providers           map[string]OAuthProvider `yaml:"providers" validate:"required,dive,keys,required,endkeys,required"`
	JWT                 struct {
		PrivateKeyFile string `yaml:"private_key_file" validate:"required,file"`
		KeyID          string `yaml:"kid" validate:"required"`
		Audience       string `yaml:"audience" validate:"required,url"`
		Expiry         string `yaml:"expiry" validate:"required"` // e.g. "15m"
	} `yaml:"jwt"`
}

type OAuthProvider struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`

	// Apple-specific fields (all required when using Apple Sign In)
	ServicesID     string `yaml:"services_id"`
	TeamID         string `yaml:"team_id"`
	KeyID          string `yaml:"key_id"`
	PrivateKeyFile string `yaml:"private_key_file"`
}

func New(path string) (cfg Config, err error) {
	defer func() {
		if err != nil {
			slog.Error(
				"failed to build config",
				"path",
				path,
				"cfg",
				cfg,
				"error",
				err,
			)
		}
	}()

	if path == "" {
		cwd, errWd := os.Getwd()
		if errWd != nil {
			err = fmt.Errorf(
				"failed to get current working directory: %w",
				err,
			)
			return
		}
		path = filepath.Join(cwd, "config.yaml")
	}

	data, errF := os.ReadFile(path)
	if errF != nil {
		err = fmt.Errorf("failed to read config file %s: %w", path, errF)
		return
	}

	if errY := yaml.Unmarshal(data, &cfg); errY != nil {
		err = fmt.Errorf("failed to parse config file %s: %w", path, errY)
		return
	}

	applyDefaults(&cfg)

	if errS := goenvsubst.Do(&cfg); errS != nil {
		err = fmt.Errorf(
			"failed to expand environment variables: %w",
			errS,
		)
		return
	}

	return cfg, validate.Struct(&cfg)
}

func applyDefaults(cfg *Config) {
	// Environment defaults to production (safer default)
	if cfg.Env == "" {
		cfg.Env = "production"
	}

	// Server defaults
	if cfg.Server.Port == "" {
		cfg.Server.Port = "8080"
	}

	// Cookie defaults
	if cfg.Cookie.Name == "" {
		cfg.Cookie.Name = "oauth_state"
	}

	// Rate limit defaults
	if cfg.RateLimit.RequestsPerMinute == 0 {
		cfg.RateLimit.RequestsPerMinute = 60
	}
	if cfg.RateLimit.CleanupInterval == 0 {
		cfg.RateLimit.CleanupInterval = 5 * time.Minute
	}
	// x_forwarded_for_index defaults to 0 (already zero value for int)

	// Logging defaults
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "text"
	}

	// Metrics defaults
	// enable and go_metrics default to false (already zero value for bool)
	if cfg.Metrics.Enable && cfg.Metrics.Path == "" {
		cfg.Metrics.Path = "/metrics"
	}
}
