package main // import "github.com/nutmegdevelopment/marathon-daemonset"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"crypto/tls"
	"strings"
	"time"
	"io"

	log "github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	mesosAgentsPath             = "/master/slaves"
	marathonAppPath             = "/v2/apps"
	defaultHTTPPostTimeout      = time.Duration(10 * time.Second)
	marathonLabelName           = "daemonset"
	marathonLabelValueSeparator = "|"
	marathonLabelAllName        = "all"
	marathonLabelAttrName       = "attr"
	workerCount                 = 2
)

var (
	config          Config
	debug           bool
	dryRun          bool
	verbose         bool
	agents          Agents
	marathonApps    MarathonApps
	updateFrequency time.Duration

	// Counter: count the number of apps updated.
	appsUpdatedCount   float64
	appsUpdatedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "marathon_daemonset",
		Subsystem: "updated",
		Name:      "apps_updated_count",
		Help:      "Number of apps updated.",
	})

	// Counter: count the number of apps updated.
	appsUpdatedErrorCount   float64
	appsUpdatedErrorCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "marathon_daemonset",
		Subsystem: "updated",
		Name:      "apps_updated_error_count",
		Help:      "Number of errors.",
	})

	client          http.Client
)

// Register the prometheus metrics.
func init() {
	prometheus.MustRegister(appsUpdatedCounter)
	prometheus.MustRegister(appsUpdatedErrorCounter)
	client = http.Client{
		Timeout: defaultHTTPPostTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: config.SkipTls},
		},
	}
}

// Agents represents the response from a Mesos /master/slaves call.
type Agents struct {
	Agents []struct {
		ID                    string                             `json:"id"`
		Hostname              string                             `json:"hostname"`
		Resources             AgentResource                      `json:"resources"`
		UsedResources         AgentResource                      `json:"used_resources"`
		OfferedResources      AgentResource                      `json:"offered_resources"`
		ReservedResources     AgentResource                      `json:"reserved_resources"`
		UnreservedResources   AgentResource                      `json:"unreserved_resources"`
		Attributes            map[string]interface{}             `json:"attributes"`
		Active                bool                               `json:"active"`
		Version               string                             `json:"version"`
		ReservedResourcesFull map[string][]AgentReservedResource `json:"reserved_resources_full"`
	} `json:"slaves"`
}

// AgentResource represents the nested resources section of a Mesos /master/slaves call.
type AgentResource struct {
	Cpus  float64 `json:"cpus"`
	Disk  float64 `json:"disk"`
	Gpus  float64 `json:"gpus"`
	Mem   float64 `json:"mem"`
	Ports string  `json:"ports"`
}

// AgentReservedResource represents the nested resserveesource section of a Mesos /master/slaves call.
type AgentReservedResource struct {
	Name   string             `json:"name"`
	Type   string             `json:"type"`
	Scalar map[string]float64 `json:"scalar"`
	Role   string             `json:"role"`
}

// MarathonApps is a map of marathon apps which require daemonset monitoring.
// The map key is the app ID.
type MarathonApps map[string]MarathonApp

// MarathonApp represents an individual app to be processed.
type MarathonApp struct {
	Attributes string
	ID         string
	Type       string
}

func HttpRequest(method string, url string, body io.Reader) (*http.Response, error) {
	req, _ := http.NewRequest(method, url, body)
	if config.Authorization != "" {
	  req.Header.Set("Authorization", config.Authorization)
	}
	return client.Do(req)
}

func HttpGet(url string) (*http.Response, error) {
	return HttpRequest("GET", url, nil)
}

func HttpPut(url string, body *bytes.Buffer) (*http.Response, error) {
	return HttpRequest("PUT", url, body)
}

// Get the apps json from Marathon.
func (m *MarathonApps) Get() ([]byte, error) {
	start := time.Now()

	url := fmt.Sprintf("%s%s", config.MarathonHost, marathonAppPath)
	log.WithFields(log.Fields{
		"url": url,
	}).Info("Reading app JSON from marathon")

	response, err := HttpGet(url)
	if err != nil {
		return nil, fmt.Errorf("Error reading app JSON from marathon: %s", err)
	}

	defer response.Body.Close()
	if response.StatusCode == 200 {
		defer response.Body.Close()
		appBody, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.WithField("error", err).Error("Unable to read the app body")
			return nil, err
		}

		log.WithFields(log.Fields{
			"time-taken": time.Since(start),
		}).Debug("MarathonApps.Get completed")
		return appBody, nil
	}
	return nil, nil
}

// Parse the apps json from Marathon into the MarathonApps map.
func (m *MarathonApps) Parse(data []byte) error {
	start := time.Now()
	// We are only interested in the 'labels', 'id' and 'instances' fields of the apps
	// JSON so we just capture that.
	var tmp struct {
		Apps []struct {
			ID        string            `json:"id"`
			Instances int               `json:"instances"`
			Labels    map[string]string `json:"labels"`
		} `json:"apps"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	// Initialize/reset the map.
	(*m) = make(map[string]MarathonApp)

	for _, app := range tmp.Apps {
		if val, ok := app.Labels[marathonLabelName]; ok {
			log.WithFields(log.Fields{
				"app-name": app.ID,
			}).Info("Found daemonset app")

			var marathonApp MarathonApp
			if val == marathonLabelAllName {
				marathonApp.Type = marathonLabelAllName
				marathonApp.ID = app.ID
				(*m)[app.ID] = marathonApp
			} else {
				marathonApp.Attributes = val
				marathonApp.Type = marathonLabelAttrName
				marathonApp.ID = app.ID
				(*m)[app.ID] = marathonApp
			}
		} else {
			log.WithField("app-id", app.ID).Debug("Found normal app")
		}
	}

	log.WithFields(log.Fields{
		"time-taken": time.Since(start),
	}).Debug("MarathonApps.Parse completed")

	return nil
}

// MarathonAppInstanceCount represents the instances value for an app.
type MarathonAppInstanceCount struct {
	Instances int
}

// Parse the app json to retrieve the instances value.
func (m *MarathonAppInstanceCount) Parse(data []byte) error {
	start := time.Now()

	// We are only interested in the 'instances' field of the app JSON so we
	// just capture that.
	var tmp struct {
		App struct {
			Instances int `json:"instances"`
		} `json:"app"`
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	m.Instances = tmp.App.Instances

	log.WithFields(log.Fields{
		"time-taken": time.Since(start),
	}).Debug("MarathonAppInstanceCount.Parse completed")

	return nil
}

func main() {
	if config.DryRun {
		log.Info("Running in dry-run mode")
	}


	// Start processing the apps in a goroutine before we start the webserver.
	go processApps()

	// Start a http server for the /metrics endpoint.
	mux := http.NewServeMux()

	// Register the prometheus /metrics endpoint.
	mux.Handle("/metrics", prometheus.Handler())

	// Register /health endpoint - intentionally empty as we just need a HTTP 200.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "We are the metric makers, And we are the dreamers of dreams... and yes, I am healthy. Thanks for asking!")
	})

	// Start the server.
	log.Printf("[INFO] Web server starting on %d", config.ServerPort)
	log.Println("[INFO] Access /health to check the health status.")
	log.Println("[INFO] Access /metrics for the Prometheus metrics.")
	http.ListenAndServe(fmt.Sprintf(":%d", config.ServerPort), mux)
}

// Continously loop and:
// - retrieve the current marathon apps.
// - if there are apps then update the agent details.
// - process the apps (sequentially).
// - sleep before repeating the process.
func processApps() {
	for {
		start := time.Now()

		appData, err := marathonApps.Get()
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Unable to get Marathon app listing")

			// Update prometheus metrics.
			appsUpdatedErrorCount++
			appsUpdatedErrorCounter.Set(appsUpdatedErrorCount)
		}
		marathonApps.Parse(appData)

		if len(marathonApps) == 0 {
			log.Warn("No daemonset apps found!")
		} else {
			log.WithFields(log.Fields{
				"app-count": len(marathonApps),
			}).Info("Found daemonset apps")

			err = agents.getAgents()
			if err != nil {
				log.WithField("error", err).Error("There was a problem getting the agents")
			} else {
				if agents.getAgentCount() == 0 {
					log.Error("No agents found - cannot process apps")
				} else {
					agents.getStatus()

					for _, app := range marathonApps {
						processApp(app, agents)
					}

					log.WithFields(log.Fields{
						"time-taken": time.Since(start),
					}).Info("ProcessApps completed")
				}
			}
		}

		// Sleep
		log.WithFields(log.Fields{
			"duration": config.UpdateFrequency,
		}).Info("Sleeping...")
		time.Sleep(config.UpdateFrequency)
	}
}

// Process an individual app.
func processApp(app MarathonApp, agents Agents) {
	start := time.Now()

	var agentCount int

	serviceInstanceCount, err := getCurrentInstanceCount(app.ID)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Unable to get the current instance count")

		// Update prometheus metrics.
		appsUpdatedErrorCount++
		appsUpdatedErrorCounter.Set(appsUpdatedErrorCount)
	} else {

		if app.Type == marathonLabelAllName {
			agentCount = agents.getAgentCount()
			log.WithFields(log.Fields{
				"service":       app.ID,
				"agent-count":   agentCount,
				"service-count": serviceInstanceCount,
			}).Info("Processing daemonset service")
		} else {
			agentCount = agents.getAgentCountByAttributes(app.Attributes)
			log.WithFields(log.Fields{
				"service":       app.ID,
				"attributes":    app.Attributes,
				"agent-count":   agentCount,
				"service-count": serviceInstanceCount,
			}).Info("Processing attribute constrained service")
		}

		if agentCount > 0 && serviceInstanceCount != agentCount {
			log.WithFields(log.Fields{
				"app-name":   app.ID,
				"found":      serviceInstanceCount,
				"expected":   agentCount,
				"pct-change": fmt.Sprintf("%.f%%", calculatePercentDifference(agentCount, serviceInstanceCount)),
				"change":     calculateChange(agentCount, serviceInstanceCount),
			}).Warn("Incorrect instance count")

			err = updateInstanceCount(agentCount, app.ID)
			if err != nil {
				// Update prometheus metrics.
				appsUpdatedErrorCount++
				appsUpdatedErrorCounter.Set(appsUpdatedErrorCount)
			}
		}
	}

	log.WithFields(log.Fields{
		"app-id":     app.ID,
		"time-taken": time.Since(start),
	}).Debug("ProcessApp completed")
}

func calculatePercentDifference(expected int, found int) float64 {
	if found < expected {
		return 100 - (float64(found) / float64(expected) * 100)
	}
	return 100 - (float64(expected) / float64(found) * 100)
}

func calculateChange(expected int, found int) string {
	if found < expected {
		return fmt.Sprintf("Adding %d instance(s)", expected-found)
	}
	return fmt.Sprintf("Removing %d instance(s)", found-expected)
}

// Get the agent details.
func (a *Agents) getAgents() error {
	start := time.Now()

	url := fmt.Sprintf("%s%s", config.MesosHost, mesosAgentsPath)
	log.WithFields(log.Fields{
		"url": url,
	}).Debug("Agents URL")

	response, err := HttpGet(url)
	if err != nil {
		return fmt.Errorf("Error reading agents JSON from mesos: %s", err)
	}

	defer response.Body.Close()
	if response.StatusCode == 200 {
		defer response.Body.Close()
		agentsBody, _ := ioutil.ReadAll(response.Body)

		err := json.Unmarshal(agentsBody, &agents)
		if err != nil {
			return fmt.Errorf("Error unmarshalling JSON from mesos: %s", err)
		}

		log.WithFields(log.Fields{
			"time-taken": time.Since(start),
		}).Debug("Agents.getAgents completed")

		return nil
	}

	return fmt.Errorf("Error reading agents JSON from mesos - non 200 response: %d", response.StatusCode)
}

// Logs details about the agents that have been found.
// Currently displays the main used ones: all, public and private.
func (a *Agents) getStatus() {
	var agentCount int

	agentCount = a.getAgentCount()
	// if agentCount == 0 {
	// 	log.Error("No agents found")
	// } else {
	if agentCount > 0 {
		log.WithFields(log.Fields{
			"agent-count": agentCount,
		}).Info("Found agents")
	} else {
		log.Warn("No agents found")
	}

	agentCount = a.getPublicAgentCount()
	if agentCount > 0 {
		log.WithFields(log.Fields{
			"agent-count": agentCount,
		}).Info("Found public agents")
	} else {
		log.Warn("No public agents found")
	}

	agentCount = a.getPrivateAgentCount()
	if agentCount > 0 {
		log.WithFields(log.Fields{
			"agent-count": agentCount,
		}).Info("Found private agents")
	} else {
		log.Warn("No private agents found")
	}
	// }
}

// Return the number of agents.
func (a *Agents) getAgentCount() int {
	return len(a.Agents)
}

// Return the number of agents with the daemonset=<attr>|<attrValue>(,<attr>|<attrValue>) label.
func (a *Agents) getAgentCountByAttributes(attributes string) int {
	agentCount := 0
	attributePairs := strings.Split(attributes, ",")
	for _, attr := range attributePairs {
		log.WithField("attr", attr).Info("Processing attribute pair")
		attributePair := strings.Split(attr, "|")
		if len(attributePair) == 2 {
			agentCount += a.getAgentCountByAttribute(attributePair[0], attributePair[1])
		} else {
			log.WithField("attr", attr).Error("The attributePair did not split into 2 parts")
		}
	}
	return agentCount
}

// Return the number of agents with the <attr>=<attrValue> attribute.
func (a *Agents) getAgentCountByAttribute(attr string, attrValue string) int {
	slaveCount := 0
	for _, v := range a.Agents {
		if v.Attributes[attr] == attrValue {
			slaveCount++
		}
	}

	// There were no matching agents.  Most likely the label is incorrect so throw a warning.
	if slaveCount == 0 {
		log.WithFields(log.Fields{
			"attr":  attr,
			"value": attrValue,
		}).Warn("Attribute matched no instances")
	}

	return slaveCount
}

// Return the number of agents with the tier=public attribute.
func (a *Agents) getPublicAgentCount() int {
	const attr = "tier"
	const attrValue = "public"
	return a.getAgentCountByAttribute(attr, attrValue)
}

// Return the number of agents with the tier=private attribute.
func (a *Agents) getPrivateAgentCount() int {
	const attr = "tier"
	const attrValue = "private"
	return a.getAgentCountByAttribute(attr, attrValue)
}

// Read in a local file.
func readFile(filename string) ([]byte, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		log.WithFields(log.Fields{
			"file": filename,
		}).Error("Unable to read the file")
		return nil, err
	}
	return b, nil
}

// Return the number of instances that Marathon is reporting for the app.
func getCurrentInstanceCount(service string) (int, error) {
	start := time.Now()

	var marathonAppInstanceCount MarathonAppInstanceCount
	url := fmt.Sprintf("%s%s%s", config.MarathonHost, marathonAppPath, service)

	response, err := HttpGet(url)
	if err != nil {
		log.Warnf("HTTP GET error: %s", err)
		return 0, err
	}

	defer response.Body.Close()
	if response.StatusCode == 200 {
		defer response.Body.Close()
		appBody, _ := ioutil.ReadAll(response.Body)

		err = marathonAppInstanceCount.Parse(appBody)
		if err != nil {
			log.Warnf("Unmarshal JSON error: %s", err)
			return -1, err
		}

		log.WithFields(log.Fields{
			"app-id":     service,
			"time-taken": time.Since(start),
		}).Debug("getCurrentInstanceCount completed")

		return marathonAppInstanceCount.Instances, nil
	}

	log.Warnf("Status code error: %s", err)
	return 0, fmt.Errorf("Non 200 status code returned: %d", response.StatusCode)
}

// Update the instances field on a Marathon app.
func updateInstanceCount(count int, service string) error {
	start := time.Now()

	instanceJSON := fmt.Sprintf("{ \"instances\": %d }", count)
	instanceJSONBuffer := bytes.NewBufferString(instanceJSON)

	url := fmt.Sprintf("%s%s%s", config.MarathonHost, marathonAppPath, service)
	log.WithFields(log.Fields{
		"url":           url,
		"instance-json": instanceJSON,
	}).Info("Posting app JSON to destination Marathon")

	if config.DryRun {
		log.Info("In dry-run mode so stopping here and not making changes")
		return nil
	}

	response, err := HttpPut(url, instanceJSONBuffer)
	if err != nil {
		log.WithFields(log.Fields{
			"error":                err,
			"json":                 instanceJSON,
			"marathon-destination": url,
		}).Error("Unable to update marathon app")
		return err
	}

	defer response.Body.Close()

	body, _ := ioutil.ReadAll(response.Body)
	if response.StatusCode == 200 {
		log.WithFields(log.Fields{
			"response-body:": string(body),
			"status-code":    response.StatusCode,
		}).Info("Marathon app update successful")

		// Update prometheus metrics.
		appsUpdatedCount++
		appsUpdatedCounter.Set(appsUpdatedCount)

		log.WithFields(log.Fields{
			"app-id":     service,
			"time-taken": time.Since(start),
		}).Debug("updateInstanceCount completed")

		return nil
	}
	if response.StatusCode == 409 {
		log.WithFields(log.Fields{
			"response-body:":  string(body),
			"status-code":     response.StatusCode,
			"destination-url": url,
		}).Warn("Conflict preventing Marathon app update")

		return fmt.Errorf("409 status code (conflict) returned: %d", response.StatusCode)
	}

	log.WithFields(log.Fields{
		"response-body:":  string(body),
		"status-code":     response.StatusCode,
		"destination-url": url,
	}).Error("Unable to update Marathon app")

	return fmt.Errorf("A non expected statuscode returned: %d", response.StatusCode)

}
