package tracker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"

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

type ParsedCatalog struct {
	Products *ProductMap `plist:"Products"`
}

type ProductMap struct {
	//SnowLeopard  *Product `plist:"zzz031-12177"` //10.6
	//Lion         *Product `plist:"031-12217"`    //10.7
	//MountainLion *Product `plist:"031-0630"`     //10.8
	//Mavericks    *Product `plist:"031-07602"`    //10.9
	Yosemite   *Product `plist:"031-30888"` //10.10
	ElCapitan  *Product `plist:"031-63178"` //10.11
	Sierra     *Product `plist:"091-22860"` //10.12
	HighSierra *Product `plist:"091-39211"` //10.13
}

type Product struct {
	Distributions *DistributionMap `plist:"Distributions"`
}

type DistributionMap struct {
	EnglishDistribution string `plist:"English"`
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
func (t *Tracker) updateOSVersionsMapFromProductMap(productMap *ProductMap, versionsInfo *VersionsInfo, lastModified time.Time) (bool, error) {
	// Get the latest version for each of the 3 latest major releases
	latestVersions := sort.StringSlice{}
	if productMap.HighSierra != nil && productMap.HighSierra.Distributions != nil && productMap.HighSierra.Distributions.EnglishDistribution != "" {
		highSierraVersion, err := t.getLatestVersion(productMap.HighSierra.Distributions.EnglishDistribution, lastModified)
		if err != nil {
			log.WithFields(log.Fields{
				"timestamp": time.Now().UnixNano(),
				"err":       err,
			}).Error("Error getting version for High Sierra")
			return false, err
		}

		if highSierraVersion != "" {
			latestVersions = append(latestVersions, highSierraVersion)
		}
	}

	if productMap.Sierra != nil && productMap.Sierra.Distributions != nil && productMap.Sierra.Distributions.EnglishDistribution != "" {
		sierraVersion, err := t.getLatestVersion(productMap.Sierra.Distributions.EnglishDistribution, lastModified)
		if err != nil {
			log.WithFields(log.Fields{
				"timestamp": time.Now().UnixNano(),
				"err":       err,
			}).Error("Error getting version for Sierra")
			return false, err
		}

		if sierraVersion != "" {
			latestVersions = append(latestVersions, sierraVersion)
		}
	}

	if productMap.ElCapitan != nil && productMap.ElCapitan.Distributions != nil && productMap.ElCapitan.Distributions.EnglishDistribution != "" {
		elCapitanVersion, err := t.getLatestVersion(productMap.ElCapitan.Distributions.EnglishDistribution, lastModified)
		if err != nil {
			log.WithFields(log.Fields{
				"timestamp": time.Now().UnixNano(),
				"err":       err,
			}).Error("Error getting version for El Capitan")
			return false, err
		}

		if elCapitanVersion != "" {
			latestVersions = append(latestVersions, elCapitanVersion)
		}
	}

	if productMap.Yosemite != nil && productMap.Yosemite.Distributions != nil && productMap.Yosemite.Distributions.EnglishDistribution != "" {
		yosemiteVersion, err := t.getLatestVersion(productMap.Yosemite.Distributions.EnglishDistribution, lastModified)
		if err != nil {
			log.WithFields(log.Fields{
				"timestamp": time.Now().UnixNano(),
				"err":       err,
			}).Error("Error getting version for Yosemite")
			return false, err
		}

		if yosemiteVersion != "" {
			latestVersions = append(latestVersions, yosemiteVersion)
		}
	}

	changed := false
	for _, latestVersion := range latestVersions {
		if latestVersion != "" {
			versionsInfo.LatestVersions[latestVersion] = true
			versionsInfo.LastModified = time.Now()
			changed = true
		}
	}
	return changed, nil
}

/**
 * Parses a response from the catalog URL into a ProductMap
 */
func (t *Tracker) parseCatalogResponse(resp *http.Response) (*ProductMap, error) {
	body, err := ioutil.ReadAll(io.Reader(resp.Body))
	if err != nil {
		log.WithFields(log.Fields{
			"timestamp": time.Now().UnixNano(),
			"err":       err,
		}).Error("Error reading response")
		return nil, err
	}

	decoder := plist.NewDecoder(bytes.NewReader(body))
	parsedCatalog := &ParsedCatalog{}
	err = decoder.Decode(parsedCatalog)
	if err != nil {
		log.WithFields(log.Fields{
			"timestamp": time.Now().UnixNano(),
			"err":       err,
			"body":      string(body),
		}).Error("Error decoding response")
		return nil, err
	}

	return parsedCatalog.Products, nil
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
		versionsInfo.LatestVersions = make(map[string]bool)
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
