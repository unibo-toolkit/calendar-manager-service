package config

import "github.com/ilyakaznacheev/cleanenv"

type Config struct {
	HTTP      HTTPConfig
	DB        DBConfig
	Scraper   ScraperConfig
	Calendar  CalendarConfig
	TTL       TTLConfig
	Snowflake SnowflakeConfig
}

type HTTPConfig struct {
	Port        string `env:"HTTP_PORT" env-default:"8082"`
	IPHeader    string `env:"HTTP_IP_HEADER"`
	Environment string `env:"ENVIRONMENT" env-default:"dev"`
	LogLevel    string `env:"LOG_LEVEL" env-default:"info"`
}

type DBConfig struct {
	Host            string `env:"DB_HOST" env-default:"localhost"`
	Port            string `env:"DB_PORT" env-default:"5432"`
	User            string `env:"DB_USER" env-default:"unibo_user"`
	Pass            string `env:"DB_PASS" env-default:"unibo_pass"`
	Name            string `env:"DB_NAME" env-default:"unibo_toolkit"`
	MaxConns        int32  `env:"DB_MAX_CONNS" env-default:"10"`
	MinConns        int32  `env:"DB_MIN_CONNS" env-default:"2"`
	MaxConnLifetime int64  `env:"DB_MAX_CONN_LIFETIME" env-default:"3600000000000"`
	MaxConnIdleTime int64  `env:"DB_MAX_CONN_IDLE_TIME" env-default:"1800000000000"`
}

type ScraperConfig struct {
	BaseURL string `env:"SCRAPER_URL" env-default:"http://localhost:8083"`
	Timeout int    `env:"SCRAPER_TIMEOUT" env-default:"30"`
}

type CalendarConfig struct {
	BaseURL  string `env:"CALENDAR_BASE_URL" env-default:"https://uniplanner.it"`
	ProdID   string `env:"CALENDAR_PROD_ID" env-default:"-//UniPlanner//Calendar//EN"`
	Timezone string `env:"CALENDAR_TIMEZONE" env-default:"Europe/Rome"`
	Domain   string `env:"CALENDAR_DOMAIN" env-default:"uniplanner.it"`
}

type TTLConfig struct {
	AnonymousDays     int `env:"TTL_ANONYMOUS_DAYS" env-default:"30"`
	AuthenticatedDays int `env:"TTL_AUTHENTICATED_DAYS" env-default:"365"`
}

type SnowflakeConfig struct {
	NodeID int64 `env:"SNOWFLAKE_NODE_ID" env-default:"1"`
}

func MustLoad() *Config {
	var cfg Config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		panic("failed to read config: " + err.Error())
	}
	return &cfg
}
