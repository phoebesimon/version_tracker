package tracker

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/hashicorp/go-version"
	log "github.com/sirupsen/logrus"
	"howett.net/plist"
)

const (
	catalogURL = "https://swscan.apple.com/content/catalogs/others/"
)

var MacCatalogs = map[string]string{
	//"10.6":  "index-leopard-snowleopard.merged-1.sucatalog",
	//"10.7":  "index-lion-snowleopard-leopard.merged-1.sucatalog",
	//"10.8":  "index-mountainlion-lion-snowleopard-leopard.merged-1.sucatalog",
	//"10.9":  "index-10.9-mountainlion-lion-snowleopard-leopard.merged-1.sucatalog",
	//"10.10": "index-10.10-10.9-mountainlion-lion-snowleopard-leopard.merged-1.sucatalog",
	//"10.11": "index-10.11-10.10-10.9-mountainlion-lion-snowleopard-leopard.merged-1.sucatalog",
	//"10.12": "index-10.12-10.11-10.10-10.9-mountainlion-lion-snowleopard-leopard.merged-1.sucatalog",
	"10.13": "index-10.13-10.12-10.11-10.10-10.9-mountainlion-lion-snowleopard-leopard.merged-1.sucatalog",
}

var VersionRegex = regexp.MustCompile(`(?ms)\s*"\s*(SU_VERS|SU_VERSION)\s*"\s*=\s*"\s*([0-9a-zA-Z\.\s]+)\s*"\s*;$`)
var TitleRegex = regexp.MustCompile(`(?ms)\s*"\s*(SU_TITLE)\s*"\s*=\s*"\s*(macOS 10|macOS Sierra|OS X)([0-9a-zA-Z\.\s]+)\s*"\s*;$`)
var DiscardRegex = regexp.MustCompile(`(?ms)\s*([0-9a-zA-Z\.\s]*)\s*(Mavericks|Recovery|Installer|Mail)\s*([0-9a-zA-Z\.\s]*)\s*$`)

var elCapitanMajor *version.Version
var sierraMajor *version.Version
var highSierraMajor *version.Version

const (
	version1011 = "10.11.0"
	version1012 = "10.12.0"
	version1013 = "10.13.0"

	versionNameElCapitan  = "ElCapitan"
	versionNameSierra     = "Sierra"
	versionNameHighSierra = "HighSierra"
)

func init() {
	var err error
	elCapitanMajor, err = version.NewVersion(version1011)
	if err != nil {
		log.WithFields(log.Fields{
			"err":     err,
			"version": version1011,
		}).Error("Could not parse static version")
		panic("Error parsing static version")
	}

	sierraMajor, err = version.NewVersion(version1012)
	if err != nil {
		log.WithFields(log.Fields{
			"err":     err,
			"version": version1012,
		}).Error("Could not parse static version")
		panic("Error parsing static version")
	}

	highSierraMajor, err = version.NewVersion(version1013)
	if err != nil {
		log.WithFields(log.Fields{
			"err":     err,
			"version": version1013,
		}).Error("Could not parse static version")
		panic("Error parsing static version")
	}
}

/**
 * Retrieves the latest version from the distribution URL
 */
func (t *Tracker) getLatestVersion(distributionURL string, lastModified time.Time) (string, error) {
	// Request the distribution info
	resp, err := t.makeRequest(distributionURL, lastModified)
	if err != nil {
		log.WithFields(log.Fields{
			"timestamp":       time.Now().UnixNano(),
			"distributionURL": distributionURL,
			"err":             err,
		}).Error("Error making request")
		return "", err
	}

	if resp.StatusCode != 200 {
		log.WithFields(log.Fields{
			"timestamp":     time.Now().Format("Mon, 2 Jan 2006 15:04:05 GMT"),
			"last_modified": lastModified.Format("Mon, 2 Jan 2006 15:04:05 GMT"),
			"status_code":   resp.StatusCode,
		}).Debug("Distribution URL has not been updated since we last pulled it; short-circuiting.")
		return "", nil
	}

	body, err := ioutil.ReadAll(io.Reader(resp.Body))
	if err != nil {
		log.WithFields(log.Fields{
			"timestamp": time.Now().UnixNano(),
			"err":       err,
		}).Error("Error reading response")
		return "", err
	}

	titleMatch := TitleRegex.FindStringSubmatch(string(body))
	if len(titleMatch) != 4 {
		log.WithFields(log.Fields{
			"timestamp":  time.Now().UnixNano(),
			"titleMatch": titleMatch,
			"err":        err,
		}).Debug("Was not a macOS version")
		return "", nil
	}

	discardMatch := DiscardRegex.FindStringSubmatch(titleMatch[3])
	if len(discardMatch) > 1 {
		log.WithFields(log.Fields{
			"timestamp":    time.Now().UnixNano(),
			"discardMatch": discardMatch,
			"err":          err,
		}).Debug("Was not a macOS version")
		return "", nil
	}

	// Pull out the version
	matches := VersionRegex.FindStringSubmatch(string(body))
	if len(matches) != 3 {
		log.WithFields(log.Fields{
			"timestamp": time.Now().UnixNano(),
			"matches":   matches,
			"err":       err,
		}).Error("Error finding latest version")
		return "", errors.New("Could not find version in distribution")
	}

	return matches[2], nil
}

/**
 * Update the version info from the product map info.
 * Returns true if it was updated, false otherwise.
 */
func (t *Tracker) updateOSVersionsMapFromProductMap(productCatalogInterface interface{}, versionsInfo *VersionsInfo, lastModified time.Time) (bool, error) {
	productCatalogMap := productCatalogInterface.(map[string]interface{})

	productsMap, ok := productCatalogMap["Products"].(map[string]interface{})
	if !ok {
		return false, errors.New("Could not parse products")
	}

	changed := false
	for key, product := range productsMap {
		productInfo, ok := product.(map[string]interface{})
		if !ok {
			continue
		}
		distributions, ok := productInfo["Distributions"].(map[string]interface{})
		if !ok {
			continue
		}

		englishDistribution, ok := distributions["English"].(string)
		if !ok {
			continue
		}

		ver, err := t.getLatestVersion(englishDistribution, lastModified)
		if err != nil {
			log.WithFields(log.Fields{
				"err": err,
				"englishDistributionURL": englishDistribution,
				"key": key,
			}).Info("Failed to get version info")
			continue
		}

		if ver == "" {
			continue
		}

		v1, err := version.NewVersion(ver)
		if err != nil {
			log.WithFields(log.Fields{
				"err": err,
				"englishDistributionURL": englishDistribution,
				"key":     key,
				"version": ver,
			}).Error("Could not parse version")
			continue
		}

		//The oldest major version we support
		if v1.LessThan(elCapitanMajor) {
			log.WithFields(log.Fields{
				"err": err,
				"englishDistributionURL": englishDistribution,
				"key":     key,
				"version": ver,
			}).Debug("Not tracked version")
			continue
		}

		if v1.GreaterThan(highSierraMajor) {
			t.mtx.Lock()
			latestHighSierraVersion, ok := versionsInfo.LatestVersions[versionNameHighSierra]
			if !ok || v1.GreaterThan(latestHighSierraVersion) {
				versionsInfo.LatestVersions[versionNameHighSierra] = v1
				versionsInfo.LastModified = time.Now()
				changed = true
			}
			t.mtx.Unlock()
		} else if v1.GreaterThan(sierraMajor) {
			t.mtx.Lock()
			latestSierraVersion, ok := versionsInfo.LatestVersions[versionNameSierra]
			if !ok || v1.GreaterThan(latestSierraVersion) {

				versionsInfo.LatestVersions[versionNameSierra] = v1
				versionsInfo.LastModified = time.Now()
				changed = true
			}
			t.mtx.Unlock()
		} else if v1.GreaterThan(elCapitanMajor) {
			t.mtx.Lock()
			latestElCapitanVersion, ok := versionsInfo.LatestVersions[versionNameElCapitan]
			if !ok || v1.GreaterThan(latestElCapitanVersion) {
				versionsInfo.LatestVersions[versionNameElCapitan] = v1
				versionsInfo.LastModified = time.Now()
				changed = true
			}
			t.mtx.Unlock()
		}
	}

	return changed, nil
}

/**
 * Parses a response from the catalog URL into a ProductMap
 */
func (t *Tracker) parseCatalogResponse(resp *http.Response) (interface{}, error) {
	body, err := ioutil.ReadAll(io.Reader(resp.Body))
	if err != nil {
		log.WithFields(log.Fields{
			"timestamp": time.Now().UnixNano(),
			"err":       err,
		}).Error("Error reading response")
		return nil, err
	}

	var parsedCatalog interface{}
	_, err = plist.Unmarshal(body, &parsedCatalog)
	if err != nil {
		log.WithFields(log.Fields{
			"timestamp": time.Now().UnixNano(),
			"err":       err,
			"body":      string(body),
		}).Error("Error unmarshalling response")
		return nil, err
	}

	return parsedCatalog, nil
}

/**
 * Attempts to request/parse info from the product catalog
 * Returns true if the it successfully updates the osVersionsMap, false otherwise
 */
func (t *Tracker) updateOSVersionsMap(url string) (bool, error) {
	// Look up the most recent version info
	var lastModified time.Time
	versionsInfo, ok := t.osVersionsMap[OSTypeMac]
	if !ok {
		lastModified = time.Time{}
	} else {
		lastModified = versionsInfo.LastModified
	}

	if versionsInfo.LatestVersions == nil {
		versionsInfo.LatestVersions = make(map[string]*version.Version)
	}

	// Request product info from the catalog
	resp, err := t.makeRequest(url, lastModified)
	if err != nil {
		log.WithFields(log.Fields{
			"timestamp": time.Now().UnixNano(),
			"err":       err,
		}).Error("Error making request")
		return false, err
	}

	// Short-circuit if we don't get a 200 (most likely means the catalog has not been updated lately)
	if resp.StatusCode != 200 {
		log.WithFields(log.Fields{
			"timestamp":     time.Now().Format("Mon, 2 Jan 2006 15:04:05 GMT"),
			"last_modified": lastModified.Format("Mon, 2 Jan 2006 15:04:05 GMT"),
			"status_code":   resp.StatusCode,
		}).Debug("Catalog has not been updated since we last pulled it; short-circuiting.")
		return false, nil
	}

	// Parse response into product info
	productMap, err := t.parseCatalogResponse(resp)
	if err != nil {
		log.WithFields(log.Fields{
			"timestamp": time.Now().UnixNano(),
			"err":       err,
		}).Error("Error parsing response")
		return false, err
	}

	return t.updateOSVersionsMapFromProductMap(productMap, versionsInfo, lastModified)
}

/**
 * Scrape the mac catalog and possibly update the osVersionsMap
 */
func (t *Tracker) scrapeForMacVersions(url string) error {
	updated, err := t.updateOSVersionsMap(url)
	if err != nil {
		log.WithFields(log.Fields{
			"timestamp": time.Now().UnixNano(),
			"err":       err,
		}).Error("Error updating versions")
		return err
	}

	if updated {
		log.WithFields(log.Fields{
			"timestamp":       time.Now().UnixNano(),
			"latest_versions": t.osVersionsMap[OSTypeMac].LatestVersions,
			"modified_at":     t.osVersionsMap[OSTypeMac].LastModified,
		}).Info("Updated version map")
	} else {
		log.WithFields(log.Fields{
			"timestamp":       time.Now().UnixNano(),
			"latest_versions": t.osVersionsMap[OSTypeMac].LatestVersions,
			"modified_at":     t.osVersionsMap[OSTypeMac].LastModified,
		}).Debug("Did not update version map")
	}

	return nil
}

func (t *Tracker) ScrapeForMacVersions() {
	wg := sync.WaitGroup{}

	for _, url := range MacCatalogs {
		wg.Add(1)

		go func(url string) {
			defer wg.Done()
			t.scrapeForMacVersions(fmt.Sprintf("%s%s", catalogURL, url))
		}(url)
	}

	wg.Wait()
}
