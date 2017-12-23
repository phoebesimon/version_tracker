package tracker

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/go-version"
	log "github.com/sirupsen/logrus"
)

const (
	OSTypeMac     = "macOS"
	OSTypeWindows = "windows"
	OSTypeLinux   = "linux"
)

type VersionsInfo struct {
	LatestVersions map[string]*version.Version
	LastModified   time.Time
}

type Tracker struct {
	interval      int
	osVersionsMap map[string]*VersionsInfo // OS Type --> latest versions/lastModified
	wg            sync.WaitGroup
	mtx           sync.RWMutex
}

func (t *Tracker) Close() {
	t.wg.Wait()
}

func (t *Tracker) Start(ctx context.Context) {
	t.wg.Add(1)
	defer t.wg.Done()

	t.mainLoop(ctx)
}

func (t *Tracker) ReadVersions(os string) *VersionsInfo {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.osVersionsMap[os]
}

func (t *Tracker) makeRequest(path string, lastModified time.Time) (*http.Response, error) {
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("If-Modified-Since", lastModified.Format("Mon, 2 Jan 2006 15:04:05 GMT"))

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		log.WithFields(log.Fields{
			"timestamp": time.Now().UnixNano(),
			"err":       err,
		}).Error("Error making request")
		return nil, err
	}

	return resp, nil
}

func (t *Tracker) mainLoop(ctx context.Context) {
	log.WithField("timestamp", time.Now().UnixNano()).Debug("Scraping...")

	t.ScrapeForMacVersions()

	log.WithField("timestamp", time.Now().UnixNano()).Debug("Finished scraping.")
	timer := time.NewTicker(time.Duration(t.interval) * time.Second)

	for {
		select {
		case <-ctx.Done():
			log.Info("Shutting down tracker.")
			timer.Stop()
			return

		case <-timer.C:
			log.WithField("timestamp", time.Now().UnixNano()).Debug("Scraping...")

			t.ScrapeForMacVersions()

			log.WithField("timestamp", time.Now().UnixNano()).Debug("Finished scraping.")
		}
	}
}

func MakeTracker(interval int) *Tracker {
	osVersionsMap := make(map[string]*VersionsInfo)

	osVersionsMap[OSTypeMac] = &VersionsInfo{
		LatestVersions: map[string]*version.Version{},
		LastModified:   time.Time{},
	}
	osVersionsMap[OSTypeWindows] = &VersionsInfo{
		LatestVersions: map[string]*version.Version{},
		LastModified:   time.Time{},
	}
	osVersionsMap[OSTypeLinux] = &VersionsInfo{
		LatestVersions: map[string]*version.Version{},
		LastModified:   time.Time{},
	}

	return &Tracker{
		interval:      interval,
		osVersionsMap: osVersionsMap,
		mtx:           sync.RWMutex{},
	}
}
