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
	l := New(dbURL, time.Hour, filePath, CountryCode)
	doTestLookup(t, l, "188.166.36.215", "")
	<-l.Ready()
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
	start := time.Now()
	l2 := New(dbURL, time.Hour, filePath, CountryCode)
	<-l2.Ready()
	assert.Less(t, time.Since(start).Nanoseconds(), 100*time.Millisecond.Nanoseconds())
	doTestLookup(t, l2, "188.166.36.215", "NL")
}

func doTestLookup(t *testing.T, l Lookup, ip string, expectedCountry string) {
	country := l.CountryCode(net.ParseIP(ip))
	assert.Equal(t, expectedCountry, country)
}

func TestLookupISP(t *testing.T) {
	filePath := "GeoIP2-ISP-Test.mmdb"
	l, err := FromFile(filePath)
	assert.NoError(t, err)

	// testLookupISP(t, l, "188.166.36.215", "DigitalOcean")
	// testLookupISP(t, l, "139.59.59.44", "Digital Ocean")
	testLookupISP(t, l, "217.164.123.118", "Emirates Telecommunications Corporation")
	// testLookupISP(t, l, "87.107.251.220", "Soroush Rasanheh Company Ltd")
	testLookupISP(t, l, "120.216.165.160", "Guangdong Mobile")
	testLookupISP(t, l, "adsfs423afsd234:2343", "")
}

func testLookupISP(t *testing.T, l Lookup, ip string, expectedISP string) {
	assert.Equal(t, expectedISP, l.ISP(net.ParseIP(ip)))
}

func TestLookupASN(t *testing.T) {
	filePath := "GeoIP2-ISP-Test.mmdb"
	l, err := FromFile(filePath)
	assert.NoError(t, err)

	testLookupASN(t, l, "217.164.123.118", "")
	testLookupASN(t, l, "120.216.165.160", "AS9808")
	testLookupASN(t, l, "adsfs423afsd234:2343", "")
}

func testLookupASN(t *testing.T, l Lookup, ip string, expectedASN string) {
	assert.Equal(t, expectedASN, l.ASN(net.ParseIP(ip)))
}

func TestFromFile(t *testing.T) {
	filePath := "./GeoIP2-ISP-Test.mmdb"
	l, err := FromFile(filePath)
	assert.NoError(t, err)

	testLookupISP(t, l, "adsfs423afsd234:2343", "")
	testLookupISP(t, l, "127.0.0.1", "")
	testLookupISP(t, l, "1.1.1.1", "")
}

func TestLookupCity(t *testing.T) {
	filePath := "./GeoIP2-City-Test.mmdb"
	l, err := FromFile(filePath)
	assert.NoError(t, err)

	testLookupCity(t, l, "81.2.69.142", "London", "United Kingdom")
}

func testLookupCity(t *testing.T, l Lookup, ip string, expectedCity, expectedCountry string) {
	city, country := l.City(net.ParseIP(ip))
	assert.Equal(t, expectedCity, city)
	assert.Equal(t, expectedCountry, country)
}

func TestLookupLatLong(t *testing.T) {
	filePath := "./GeoIP2-City-Test.mmdb"
	l, err := FromFile(filePath)
	assert.NoError(t, err)

	testLookupLatLong(t, l, "81.2.69.142", 51.5142, -0.0931)
}

func testLookupLatLong(t *testing.T, l Lookup, ip string, expectedLat, expectedLong float64) {
	lat, long := l.LatLong(net.ParseIP(ip))
	assert.Equal(t, expectedLat, lat)
	assert.Equal(t, expectedLong, long)
}
