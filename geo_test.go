package geo

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLookupCountry(t *testing.T) {
	licenseKey := os.Getenv("MAXMIND_LICENSE_KEY")
	if licenseKey == "" {
		t.Skip("require envvar MAXMIND_LICENSE_KEY")
	}
	dbURL := fmt.Sprintf("https://download.maxmind.com/app/geoip_download?license_key=%s&edition_id=GeoLite2-Country&suffix=tar.gz", licenseKey)

	filePath := "GeoLite2-Country.mmdb"
	defer os.Remove(filePath)
	l := New(dbURL, time.Hour, filePath)
	time.Sleep(20 * time.Second) // wait long enough to load database remotely
	_, err := os.Stat(filePath)
	assert.NoError(t, err, "should have cached the database locally")
	doTestLookup(t, l, "188.166.36.215", "NL")
	doTestLookup(t, l, "188.166.36.215", "NL")
	doTestLookup(t, l, "139.59.59.44", "IN")
	doTestLookup(t, l, "139.59.59.44", "IN")
	doTestLookup(t, l, "45.55.177.174", "US")
	doTestLookup(t, l, "139.59.59.44", "IN")
	doTestLookup(t, l, "188.166.36.215", "NL")
	doTestLookup(t, l, "217.164.123.118", "AE")
	doTestLookup(t, l, "87.107.251.220", "IR")
	doTestLookup(t, l, "120.216.165.160", "CN")
	doTestLookup(t, l, "adsfs423afsd234:2343", "")
	doTestLookup(t, l, "adsfs423afsd234:2343", "")

	// Make sure that when the local file exists, lookup works immediately.
	l2 := New(dbURL, time.Hour, filePath)
	time.Sleep(100 * time.Millisecond)
	doTestLookup(t, l2, "188.166.36.215", "NL")
}

func doTestLookup(t *testing.T, l Lookup, ip string, expectedCountry string) {
	country := l.CountryCode(net.ParseIP(ip))
	assert.Equal(t, expectedCountry, country)
}
