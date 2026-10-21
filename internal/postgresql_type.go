package python

import (
	"fmt"
	"os"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/sqlc-dev/plugin-sdk-go/sdk"
)

func postgresType(req *plugin.GenerateRequest, col *plugin.Column) string {
	columnType := sdk.DataType(col.GetType())

	switch columnType {
	case "serial",
		"serial4",
		"pg_catalog.serial4",
		"bigserial",
		"serial8",
		"pg_catalog.serial8",
		"smallserial",
		"serial2",
		"pg_catalog.serial2",
		"integer",
		"int",
		"int4",
		"pg_catalog.int4",
		"bigint",
		"int8",
		"pg_catalog.int8",
		"smallint",
		"int2",
		"pg_catalog.int2":
		return "int"
	case "float", "double precision", "float8", "pg_catalog.float8", "real", "float4", "pg_catalog.float4":
		return "float"
	case "numeric", "pg_catalog.numeric", "money":
		return "decimal.Decimal"
	case "boolean", "bool", "pg_catalog.bool":
		return "bool"
	case "json", "jsonb":
		return pythonAnyType
	case "bytea", "blob", "pg_catalog.bytea":
		return "memoryview"
	case "date":
		return "datetime.date"
	case "pg_catalog.time", "pg_catalog.timetz":
		return "datetime.time"
	case "pg_catalog.timestamp", "pg_catalog.timestamptz", "timestamptz":
		return "datetime.datetime"
	case "interval", "pg_catalog.interval":
		return "datetime.timedelta"
	case "text", "pg_catalog.varchar", "pg_catalog.bpchar", "string", "citext":
		return pythonStrType
	case "uuid":
		return "uuid.UUID"
	case "inet", "cidr", "macaddr", "macaddr8":
		// psycopg2 does have support for ipaddress objects, but it is not enabled by default
		//
		// https://www.psycopg.org/docs/extras.html#adapt-network
		return pythonStrType
	case "ltree", "lquery", "ltxtquery":
		return pythonStrType
	default:
		for _, schema := range req.GetCatalog().GetSchemas() {
			if schema.GetName() == postgresCatalog || schema.GetName() == informationSchema {
				continue
			}
			for _, enum := range schema.GetEnums() {
				if columnType == enum.GetName() {
					if schema.GetName() == req.GetCatalog().GetDefaultSchema() {
						return "models." + modelName(enum.GetName())
					}
					return "models." + modelName(schema.GetName()+"_"+enum.GetName())
				}
			}
		}
		fmt.Fprintf(os.Stderr, "unknown PostgreSQL type: %s\n", columnType)
		return pythonAnyType
	}
}
