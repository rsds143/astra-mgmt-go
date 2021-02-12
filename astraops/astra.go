/**
Copyright 2021 Ryan Svihla

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/
package astraops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Authenticate returns a token from the service account
func Authenticate(clientName, clientId, clientSecret string) (*AuthenticatedClient, error) {
	url := "https://api.astra.datastax.com/v2/authenticateServiceAccount"
	c := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxConnsPerHost:     10,
			MaxIdleConnsPerHost: 10,
			Dial: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 10 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	payload := map[string]interface{}{
		"clientName":   clientName,
		"clientId":     clientId,
		"clientSecret": clientSecret,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return &AuthenticatedClient{}, fmt.Errorf("unable to marshal JSON object with: %w", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return &AuthenticatedClient{}, fmt.Errorf("failed creating request with: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.Do(req)
	if err != nil {
		return &AuthenticatedClient{}, fmt.Errorf("failed listing databases with: %w", err)
	}
	var tokenResponse map[string]interface{}
	if res.StatusCode != 200 {
		err = json.NewDecoder(res.Body).Decode(&tokenResponse)
		if err != nil {
			return &AuthenticatedClient{}, fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return &AuthenticatedClient{}, fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, tokenResponse["errors"])
	}
	err = json.NewDecoder(res.Body).Decode(&tokenResponse)
	if err != nil {
		return &AuthenticatedClient{}, fmt.Errorf("unable to decode response with error: %w", err)
	}
	if token, ok := tokenResponse["token"]; !ok {
		return &AuthenticatedClient{}, fmt.Errorf("unable to find token in json: %s", payload)
	} else {
		return &AuthenticatedClient{
			client: c,
			token:  fmt.Sprintf("Bearer %s", token),
		}, nil
	}
}

// AuthenticatedClient has a token and the methods to query the Astra DevOps API
type AuthenticatedClient struct {
	token  string
	client *http.Client
}

const serviceUrl = "https://api.astra.datastax.com/v2/databases"

func (a *AuthenticatedClient) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", a.token)
	req.Header.Set("Content-Type", "application/json")
}

// ListDb find all databases that match the parameters
// include, provider, startingAfter and limit are all optional
func (a *AuthenticatedClient) ListDb(include string, provider string, startingAfter string, limit int32) ([]DataBase, error) {
	var dbs []DataBase
	req, err := http.NewRequest("GET", serviceUrl, http.NoBody)
	if err != nil {
		return dbs, fmt.Errorf("failed creating request with: %w", err)
	}
	a.setHeaders(req)
	q := req.URL.Query()
	if len(include) > 0 {
		q.Add("include", include)
	}
	if len(provider) > 0 {
		q.Add("provider", provider)
	}
	if len(startingAfter) > 0 {
		q.Add("starting_after", startingAfter)
	}
	if limit > 0 {
		q.Add("limit", strconv.FormatInt(int64(limit), 10))
	}
	req.URL.RawQuery = q.Encode()
	res, err := a.client.Do(req)
	if err != nil {
		return dbs, fmt.Errorf("failed listing databases with: %w", err)
	}
	if res.StatusCode != 200 {
		var resObj map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resObj)
		if err != nil {
			return []DataBase{}, fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return []DataBase{}, fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, resObj["errors"])
	}
	err = json.NewDecoder(res.Body).Decode(&dbs)
	if err != nil {
		return []DataBase{}, fmt.Errorf("unable to decode response with error: %w", err)
	}
	return dbs, nil
}

// CreateDb creates a database in Astra, all fields are required
func (a *AuthenticatedClient) CreateDb(name, keyspace, region, dbUser, dbPass, tier string, capacityUnits int) error {
	createDb := map[string]interface{}{
		"name":          name,
		"keyspace":      keyspace,
		"capacityUnits": capacityUnits,
		"region":        region,
		"user":          dbUser,
		"password":      dbPass,
		"tier":          tier,
	}
	body, err := json.Marshal(createDb)
	if err != nil {
		return fmt.Errorf("unable to marshall create db json with: %w", err)
	}
	req, err := http.NewRequest("POST", serviceUrl, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed creating request with: %w", err)
	}
	a.setHeaders(req)
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed creating database with: %w", err)
	}
	if res.StatusCode != 200 {
		var resObj map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resObj)
		if err != nil {
			return fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, resObj["errors"])
	}
	return nil
}

// FinDb finds the database at the specified id
func (a *AuthenticatedClient) FindDb(id string) (DataBase, error) {
	var dbs DataBase
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", serviceUrl, id), http.NoBody)
	if err != nil {
		return dbs, fmt.Errorf("failed creating request to find db with id %s with: %w", id, err)
	}
	a.setHeaders(req)
	res, err := a.client.Do(req)
	if err != nil {
		return dbs, fmt.Errorf("failed get database id %s with: %w", id, err)
	}
	if res.StatusCode != 200 {
		var resObj map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resObj)
		if err != nil {
			return DataBase{}, fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return DataBase{}, fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, resObj["errors"])
	}
	err = json.NewDecoder(res.Body).Decode(&dbs)
	if err != nil {
		return DataBase{}, fmt.Errorf("unable to decode response with error: %w", err)
	}
	return dbs, nil
}

// AddKeyspaceToDb adds a keyspace to the database at the specified id
func (a *AuthenticatedClient) AddKeyspaceToDb(dbId, keyspaceName string) error {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s/keyspaces/%s", serviceUrl, dbId, keyspaceName), http.NoBody)
	if err != nil {
		return fmt.Errorf("failed creating request to add keyspace to db with id %s with: %w", dbId, err)
	}
	a.setHeaders(req)
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add keyspace to db id %s with: %w", dbId, err)
	}
	if res.StatusCode != 200 {
		var resObj map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resObj)
		if err != nil {
			return fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, resObj["errors"])
	}
	return nil
}

// GetSecureBundle finds the secure bundle connection information for the database at the specified id
func (a *AuthenticatedClient) GetSecureBundle(id string) (SecureBundle, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s/secureBundleURL", serviceUrl, id), http.NoBody)
	if err != nil {
		return SecureBundle{}, fmt.Errorf("failed creating request to get secure bundle for db with id %s with: %w", id, err)
	}
	a.setHeaders(req)
	res, err := a.client.Do(req)
	if err != nil {
		return SecureBundle{}, fmt.Errorf("failed get secure bundle for database id %s with: %w", id, err)
	}
	if res.StatusCode != 200 {
		var resObj map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resObj)
		if err != nil {
			return SecureBundle{}, fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return SecureBundle{}, fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, resObj["errors"])
	}
	var sb SecureBundle
	err = json.NewDecoder(res.Body).Decode(&sb)
	if err != nil {
		return SecureBundle{}, fmt.Errorf("unable to decode response with error: %w", err)
	}
	return sb, nil
}

// Terminate deletes the database at the specified id, preparedStateOnly can be left to false in almost all cases
// and is included only for completeness
func (a *AuthenticatedClient) Terminate(id string, preparedStateOnly bool) error {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s/terminate", serviceUrl, id), http.NoBody)
	if err != nil {
		return fmt.Errorf("failed creating request to terminate db with id %s with: %w", id, err)
	}
	a.setHeaders(req)
	q := req.URL.Query()
	q.Add("preparedStateOnly", strconv.FormatBool(preparedStateOnly))
	req.URL.RawQuery = q.Encode()
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to terminate database id %s with: %w", id, err)
	}
	if res.StatusCode != 200 {
		var resObj map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resObj)
		if err != nil {
			return fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, resObj["errors"])
	}
	return nil
}

// Park parks the database at the specified id
func (a *AuthenticatedClient) Park(id string) error {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s/park", serviceUrl, id), http.NoBody)
	if err != nil {
		return fmt.Errorf("failed creating request to park db with id %s with: %w", id, err)
	}
	a.setHeaders(req)
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to park database id %s with: %w", id, err)
	}
	if res.StatusCode != 200 {
		var resObj map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resObj)
		if err != nil {
			return fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, resObj["errors"])
	}
	return nil
}

// UnPark unparks the database at the specified id
func (a *AuthenticatedClient) UnPark(id string) error {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s/unpark", serviceUrl, id), http.NoBody)
	if err != nil {
		return fmt.Errorf("failed creating request to unpark db with id %s with: %w", id, err)
	}
	a.setHeaders(req)
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to unpark database id %s with: %w", id, err)
	}
	if res.StatusCode != 200 {
		var resObj map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resObj)
		if err != nil {
			return fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, resObj["errors"])
	}
	return nil
}

// Resize changes the storage size for the database at the specified id
func (a *AuthenticatedClient) Resize(id string, capacityUnits int32) error {
	body := fmt.Sprintf("{\"capacityUnits\":%d}", capacityUnits)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s/resize", serviceUrl, id), bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("failed creating request to unpark db with id %s with: %w", id, err)
	}
	a.setHeaders(req)
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to unpark database id %s with: %w", id, err)
	}
	if res.StatusCode != 200 {
		var resObj map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resObj)
		if err != nil {
			return fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, resObj["errors"])
	}
	return nil
}

// ResetPassword changes the password for the database at the specified id
func (a *AuthenticatedClient) ResetPassword(id, username, password string) error {
	body := fmt.Sprintf("{\"username\":\"%s\",\"password\":\"%s\"}", username, password)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s/resetPassword", serviceUrl, id), bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("failed creating request to reset password for db with id %s with: %w", id, err)
	}
	a.setHeaders(req)
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reset password for database id %s with: %w", id, err)
	}
	if res.StatusCode != 200 {
		var resObj map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resObj)
		if err != nil {
			return fmt.Errorf("unable to decode error response with error: %w", err)
		}
		return fmt.Errorf("expected status code 200 but had: %v error was %v", res.StatusCode, resObj["errors"])
	}
	return nil
}

// Info is some database meta data info
type Info struct {
	Name                string         `json:"name"`
	Keyspace            string         `json:"keyspace"`
	CloudProvider       string         `json:"cloudProvider"`
	Tier                string         `json:"tier"`
	CapacityUnits       int            `json:"capacityUnits"`
	Region              string         `json:"region"`
	User                string         `json:"user"`
	Password            string         `json:"password"`
	AdditionalKeyspaces []string       `json:"additionalKeyspaces"`
	Cost                map[string]int `json:"cost"`
}

// Storage is the storage information for the cluster
type Storage struct {
	NodeCount         int `json:"nodeCount"`
	ReplicationFactor int `json:"replicationFactor"`
	TotalStorage      int `json:"totalStorage"`
	UsedStorage       int `json:"usedStorage"`
}

// DataBase is the returned data from the Astra DevOps API
type DataBase struct {
	Id               string   `json:"id"`
	OrgId            string   `json:"orgId"`
	OwnerId          string   `json:"ownerId"`
	Info             Info     `json:"info"`
	CreationTime     string   `json:"creationTime"`
	TerminationTime  string   `json:"terminationTime"`
	Status           string   `json:"status"`
	Storage          Storage  `json:"storage"`
	AvailableActions []string `json:"availableActions"`
	Message          string   `json:"message"`
	StudioUrl        string   `json:"studioUrl"`
	GrafanaUrl       string   `json:"grafanaUrl"`
	CqlshUrl         string   `json:"cqlshUrl"`
	GraphqlUrl       string   `json:"graphUrl"`
	DataEndpointUrl  string   `json:"dataEndpointUrl"`
}

// SecureBundle connection information
type SecureBundle struct {
	DownloadURL               string `json:"downloadURL"`
	DownloadURLInternal       string `json:"downloadURLInternal"`
	DownloadURLMigrationProxy string `json:"downloadURLMigrationProxy"`
}
