// Package drivers registers the storage engine selected by build tags.
// The untagged build links bbolt only. Pass -tags sqlite, mysql, or postgres
// for one SQL driver. MariaDB uses the mysql tag. Do not combine those tags:
// the canary binary is one driver.
package drivers
