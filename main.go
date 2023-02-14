package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type RequestGroup1d struct {
	Dimensions struct {
		Date string `json:"date"`
	} `json:"dimensions"`
	Sum struct {
		Requests  int `json:"requests"`
		PageViews int `json:"pageViews"`
	} `json:"sum"`
	Uniq struct {
		Uniques int `json:"uniques"`
	} `json:"uniq"`
}

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func main() {

	log.SetOutput(os.Stderr)

	token := os.Getenv("CF_TOKEN")
	if token == "" {
		panic("CF_TOKEN is not set")
	}

	zoneId := os.Getenv("CF_ZONE_ID")
	if token == "" {
		panic("CF_ZONE_ID is not set")
	}

	csvFile, err := os.OpenFile("access.csv", os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		panic(err)
	}

	defer csvFile.Close()

	csvReader := csv.NewReader(csvFile)

	records, err := csvReader.ReadAll()
	if err != nil {
		panic(err)
	}

	// seek csvfile to the end
	csvFile.Seek(0, 2)
	csvWriter := csv.NewWriter(csvFile)

	today := time.Now()

	startDate := ""

	if len(records) == 0 {
		// if no records, start from 1 year ago
		startDate = today.Add(-31539600 * time.Second).Format("2006-01-02")
		// write header to csv file
		csvWriter.Write([]string{"date", "requests", "pageViews", "uniques"})
		csvWriter.Flush()
		log.Printf("Empty CSV detected, starting from %s", startDate)
	} else {
		startDate = records[len(records)-1][0]
		log.Printf("Existing CSV file detected, starting from %s", startDate)
	}

	query := `
	query($zoneId: string, $date: string) {
		viewer {
			zones(filter: {zoneTag: $zoneId }) {
				httpRequests1dGroups(
					filter: {
						date_gt : $date
					}
					orderBy: [date_ASC]
					limit: 10000
					) {
					dimensions { date }
					sum {
						requests,
						pageViews,
					}
					uniq {
						uniques
					}
				}
			}
		}
	}
`

	reqBytes, err := json.Marshal(&GraphQLRequest{
		Query: query,
		Variables: map[string]interface{}{
			"zoneId": zoneId,
			"date":   startDate,
		},
	})

	fmt.Println(string(reqBytes))

	if err != nil {
		panic(err)
	}

	req, err := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/graphql", bytes.NewReader(reqBytes))
	if err != nil {
		panic(err)
	}

	req.Header.Add("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("GraphQL request failed", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		panic("bad status code")
	}

	responseHolder := struct {
		Data struct {
			Viewer struct {
				Zones []struct {
					HttpRequests1dGroups []RequestGroup1d `json:"httpRequests1dGroups"`
				} `json:"zones"`
			} `json:"viewer"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}{}

	err = json.NewDecoder(resp.Body).Decode(&responseHolder)
	if err != nil {
		panic(err)
	}

	if len(responseHolder.Errors) > 0 {
		panic(responseHolder.Errors[0].Message)
	}

	for _, item := range responseHolder.Data.Viewer.Zones {
		for i, group := range item.HttpRequests1dGroups {
			if i != len(item.HttpRequests1dGroups)-1 {
				log.Println(group.Dimensions.Date, group.Sum.Requests, group.Sum.PageViews, group.Uniq.Uniques)
				csvWriter.Write([]string{group.Dimensions.Date, fmt.Sprintf("%d", group.Sum.Requests), fmt.Sprintf("%d", group.Sum.PageViews), fmt.Sprintf("%d", group.Uniq.Uniques)})
			}
		}
	}

	csvWriter.Flush()
}
