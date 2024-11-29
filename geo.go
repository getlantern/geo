// Package geo provides functionality for looking up country code and ISP name
// of the given IP address.
package geo

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	geoip2 "github.com/oschwald/geoip2-golang"

	"github.com/getlantern/golog"
	"github.com/getlantern/keepcurrent"
)

var (
	log = golog.LoggerFor("geo")
)

var (
	testIP = net.ParseIP("8.8.8.8") // Google DNS
)

type Lookup interface {
	CountryLookup
	ISPLookup
	CityLookup
	LatLongLookup
	// Ready returns a channel which is closed when the lookup is ready to
	// serve requests.
	Ready() <-chan struct{}
}

// CountryLookup allows looking up the country for an IP address
type CountryLookup interface {
	// CountryCode looks up the 2 digit ISO 3166 country code in upper case for
	// the given IP address and returns "" if there was an error.
	CountryCode(ip net.IP) string
}

// ISPLookup allows looking up ISP information for an IP address
type ISPLookup interface {
	// ISP looks up the ISP name for the given IP address and returns "" if there was an error.
	ISP(ip net.IP) string

	// ASN looks up the ASN number (e.g. AS62041) for the given IP address and returns "" if there was an error.
	ASN(ip net.IP) string
}

type CityLookup interface {
	// City looks up the city and country for the given IP address and returns "", "" if there was an error.
	City(ip net.IP) (string, string)
}

type LatLongLookup interface {
	// LatLong looks up the latitude and longitude for the given IP address and returns 0, 0 if there was an error.
	LatLong(ip net.IP) (float64, float64)
}

// NoLookup is a Lookup implementation which always return empty result.
type NoLookup struct{}

func (l NoLookup) CountryCode(ip net.IP) string         { return "" }
func (l NoLookup) ISP(ip net.IP) string                 { return "" }
func (l NoLookup) ASN(ip net.IP) string                 { return "" }
func (l NoLookup) City(ip net.IP) (string, string)      { return "", "" }
func (l NoLookup) LatLong(ip net.IP) (float64, float64) { return 0, 0 }
func (l NoLookup) Ready() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

var _ Lookup = NoLookup{}

type lookup struct {
	runner    *keepcurrent.Runner
	db        atomic.Value
	ready     chan struct{}
	readyOnce sync.Once
}

// New constructs a new Lookup from the MaxMind GeoLite2 Country database
// fetched from the given URL and keeps in sync with it every syncInterval. If filePath
// is not empty, it saves the database file to filePath and uses the file if
// available.
// lookupForValidation is a function that we call to validate new databases as they're loaded.
func New(dbURL string, syncInterval time.Duration, filePath string, lookupForValidation func(*geoip2.Reader, net.IP) (string, error)) *lookup {
	return FromWeb(dbURL, "GeoLite2-Country.mmdb", syncInterval, filePath, lookupForValidation)
}

// FromWeb is same as New but allows downloading a different MaxMind database
// lookupForValidation is a function that we call to validate new databases as they're loaded.
func FromWeb(dbURL string, nameInTarball string, syncInterval time.Duration, filePath string, lookupForValidation func(*geoip2.Reader, net.IP) (string, error)) *lookup {
	log.Debugf("Will look for updates at %v", dbURL)
	source := keepcurrent.FromTarGz(keepcurrent.FromWeb(dbURL), nameInTarball)
	chDB := make(chan []byte)
	dest := keepcurrent.ToChannel(chDB)
	var runner *keepcurrent.Runner
	if filePath != "" {
		log.Debugf("Will save database to %v", filePath)
		runner = keepcurrent.NewWithValidator(
			validator(lookupForValidation),
			source,
			keepcurrent.ToFile(filePath),
			dest,
		)
	} else {
		runner = keepcurrent.NewWithValidator(
			validator(lookupForValidation),
			source,
			dest,
		)
	}

	v := &lookup{runner: runner, ready: make(chan struct{})}
	go func() {
		for data := range chDB {
			log.Debugf("Got database of size %v", len(data))
			db, err := geoip2.FromBytes(data)
			if err != nil {
				log.Errorf("Error loading geo database: %v", err)
			} else {
				v.db.Store(db)
				v.readyOnce.Do(func() { close(v.ready) })
			}
		}
	}()
	if filePath != "" {
		runner.InitFrom(keepcurrent.FromFile(filePath))
	}

	runner.OnSourceError = keepcurrent.ExpBackoffThenFail(time.Minute, 30, func(err error) {
		log.Errorf("Unrecoverable error fetching geo database: %v", err)
	})
	runner.Start(syncInterval)
	return v
}

// LatLongFromWeb is same as New but allows downloading a different MaxMind database for lat/long lookup
// lookupForValidation is a function that we call to validate new databases as they're loaded.
func LatLongFromWeb(dbURL string, nameInTarball string, syncInterval time.Duration, filePath string, lookupForValidation func(*geoip2.Reader, net.IP) (float64, float64, error)) *lookup {
	log.Debugf("Will look for updates at %v", dbURL)
	source := keepcurrent.FromTarGz(keepcurrent.FromWeb(dbURL), nameInTarball)
	chDB := make(chan []byte)
	dest := keepcurrent.ToChannel(chDB)
	var runner *keepcurrent.Runner
	if filePath != "" {
		log.Debugf("Will save database to %v", filePath)
		runner = keepcurrent.NewWithValidator(
			latLongValidator(lookupForValidation),
			source,
			keepcurrent.ToFile(filePath),
			dest,
		)
	} else {
		runner = keepcurrent.NewWithValidator(
			latLongValidator(lookupForValidation),
			source,
			dest,
		)
	}

	v := &lookup{runner: runner, ready: make(chan struct{})}
	go func() {
		for data := range chDB {
			log.Debugf("Got database of size %v", len(data))
			db, err := geoip2.FromBytes(data)
			if err != nil {
				log.Errorf("Error loading geo database: %v", err)
			} else {
				v.db.Store(db)
				v.readyOnce.Do(func() { close(v.ready) })
			}
		}
	}()
	if filePath != "" {
		runner.InitFrom(keepcurrent.FromFile(filePath))
	}

	runner.OnSourceError = keepcurrent.ExpBackoffThenFail(time.Minute, 30, func(err error) {
		log.Errorf("Unrecoverable error fetching geo database: %v", err)
	})
	runner.Start(syncInterval)
	return v
}

// FromFile uses the local database file for lookup
func FromFile(filePath string) (*lookup, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	db, err := geoip2.FromBytes(data)
	if err != nil {
		return nil, err
	}
	v := &lookup{}
	v.db.Store(db)

	return v, nil
}

func (l *lookup) Ready() <-chan struct{} {
	return l.ready
}

func (l *lookup) CountryCode(ip net.IP) string {
	if db := l.db.Load(); db != nil {
		countryCode, err := CountryCode(db.(*geoip2.Reader), ip)
		if err != nil {
			return ""
		}
		return countryCode
	}
	return ""
}

func City(db *geoip2.Reader, ip net.IP) (string, string, error) {
	geoData, err := db.City(ip)
	if err != nil {
		return "", "", err
	}
	return geoData.City.Names["en"], geoData.Country.Names["en"], nil
}

func (l *lookup) City(ip net.IP) (string, string) {
	if db := l.db.Load(); db != nil {
		city, country, err := City(db.(*geoip2.Reader), ip)
		if err != nil {
			return "", ""
		}
		return city, country
	}
	return "", ""
}

func LatLong(db *geoip2.Reader, ip net.IP) (float64, float64, error) {
	geoData, err := db.City(ip)
	if err != nil {
		return 0, 0, err
	}
	return geoData.Location.Latitude, geoData.Location.Longitude, nil
}

func (l *lookup) LatLong(ip net.IP) (float64, float64) {
	if db := l.db.Load(); db != nil {
		lat, long, err := LatLong(db.(*geoip2.Reader), ip)
		if err != nil {
			return 0, 0
		}
		return lat, long
	}
	return 0, 0
}

func CountryCode(db *geoip2.Reader, ip net.IP) (string, error) {
	geoData, err := db.Country(ip)
	if err != nil {
		return "", log.Errorf("db failed for ip %s to get country error is: %v", ip.String(), err)
	}

	if geoData.Country.IsoCode == "" {
		return geoData.Country.IsoCode, log.Error(fmt.Errorf("empty country iso code for IP: %s", ip.String()))
	}
	return geoData.Country.IsoCode, nil
}

func (l *lookup) ISP(ip net.IP) string {
	if db := l.db.Load(); db != nil {
		isp, err := ISP(db.(*geoip2.Reader), ip)
		if err != nil {
			return ""
		}
		return isp
	}
	return ""
}

func ISP(db *geoip2.Reader, ip net.IP) (string, error) {
	geoData, err := db.ISP(ip)
	if err != nil {
		return "", err
	}
	return geoData.ISP, nil
}

func (l *lookup) ASN(ip net.IP) string {
	if db := l.db.Load(); db != nil {
		isp, err := ASN(db.(*geoip2.Reader), ip)
		if err != nil {
			return ""
		}
		return isp
	}
	return ""
}

func ASN(db *geoip2.Reader, ip net.IP) (string, error) {
	geoData, err := db.ASN(ip)
	if err != nil {
		return "", err
	}
	if geoData.AutonomousSystemNumber == 0 {
		return "", nil
	}
	return fmt.Sprintf("AS%d", geoData.AutonomousSystemNumber), nil
}

func validator(lookupForValidation func(db *geoip2.Reader, ip net.IP) (string, error)) func([]byte) error {
	return func(data []byte) error {
		db, err := geoip2.FromBytes(data)
		if err != nil {
			return log.Errorf("db failed to open: %v", err)
		}
		_, err = lookupForValidation(db, testIP)
		if err != nil {
			return log.Errorf("db failed to validate: %v", err)
		}
		log.Debug("db validated")
		return nil
	}
}

func latLongValidator(lookupForValidation func(db *geoip2.Reader, ip net.IP) (float64, float64, error)) func([]byte) error {
	return func(data []byte) error {
		db, err := geoip2.FromBytes(data)
		if err != nil {
			return log.Errorf("db failed to open: %v", err)
		}
		_, _, err = lookupForValidation(db, testIP)
		if err != nil {
			return log.Errorf("db failed to validate: %v", err)
		}
		log.Debug("db validated")
		return nil
	}
}
