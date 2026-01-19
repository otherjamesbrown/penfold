module github.com/otherjamesbrown/penfold/cmd/penf

go 1.24.0

require (
	github.com/jackc/pgx/v5 v5.8.0
	github.com/otherjamesbrown/penfold/pkg v0.0.0
	github.com/otherjamesbrown/penfold/services/search v0.0.0-20260119143758-3fd59874db2f
	github.com/redis/go-redis/v9 v9.17.2
	github.com/rs/zerolog v1.34.0
	github.com/spf13/cobra v1.8.1
	github.com/stretchr/testify v1.11.1
	golang.org/x/term v0.37.0
	google.golang.org/grpc v1.77.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/otherjamesbrown/penfold/pkg => ../../pkg

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.39.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)
