// Package geo provides functionality for looking up country codes based on
// IP addresses.
package geo

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/keepcurrent"
	geoip2 "github.com/oschwald/geoip2-golang"
)

var (
	log = golog.LoggerFor("geo")
)

// Lookup allows looking up the country for an IP address
type Lookup interface {
	// CountryCode looks up the 2 digit ISO 3166 country code in upper case for
	// the given IP address and returns "" if there was an error.
	CountryCode(ip net.IP) string
}

// NoLookup is a Lookup implementation which always return empty country code.
type NoLookup struct{}

func (l NoLookup) CountryCode(ip net.IP) string { return "" }

type lookup struct {
	runner *keepcurrent.Runner
	db     atomic.Value
}

// New constructs a new Lookup from the MaxMind GeoLite2 Country database
// fetched from the given URL and keeps in sync with it every syncInterval. If filePath
// is not empty, it saves the database file to filePath and uses the file if
// available.
func New(dbURL string, syncInterval time.Duration, filePath string) Lookup {
	source := keepcurrent.FromTarGz(keepcurrent.FromWeb(dbURL), "GeoLite2-Country.mmdb")
	chDB := make(chan []byte)
	dest := keepcurrent.ToChannel(chDB)
	var runner *keepcurrent.Runner
	if filePath != "" {
		runner = keepcurrent.New(source, keepcurrent.ToFile(filePath), dest)
	} else {
		runner = keepcurrent.New(source, dest)
	}

	v := &lookup{runner: runner}
	go func() {
		for data := range chDB {
			db, err := geoip2.FromBytes(data)
			if err != nil {
				log.Errorf("Error loading geo database: %v", err)
			} else {
				v.db.Store(db)
			}
		}
	}()
	if filePath != "" {
		runner.InitFrom(keepcurrent.FromFile(filePath))
	}

	runner.OnSourceError = keepcurrent.ExpBackoffThenFail(time.Minute, 5, func(err error) {
		log.Errorf("Error fetching geo database: %v", err)
	})
	runner.Start(syncInterval)
	return v
}

func (l *lookup) CountryCode(ip net.IP) string {
	if db := l.db.Load(); db != nil {
		geoData, err := db.(*geoip2.Reader).Country(ip)
		if err != nil {
			log.Debugf("Unable to look up ip address %s: %s", ip, err)
			return ""
		}
		return geoData.Country.IsoCode
	}
	return ""
}
