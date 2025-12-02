package datasources

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ComtradeClient interfaces with UN Comtrade API for real trade data
// Documentation: https://comtradeapi.un.org/
type ComtradeClient struct {
	BaseURL string
	Client  *http.Client
}

func NewComtradeClient() *ComtradeClient {
	return &ComtradeClient{
		BaseURL: "https://comtradeapi.un.org/data/v1",
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TradeFlow represents bilateral trade between two countries
type TradeFlow struct {
	ReporterCode    string  `json:"reporterCode"`
	ReporterDesc    string  `json:"reporterDesc"`
	PartnerCode     string  `json:"partnerCode"`
	PartnerDesc     string  `json:"partnerDesc"`
	FlowCode        string  `json:"flowCode"` // "M" = Import, "X" = Export
	FlowDesc        string  `json:"flowDesc"`
	CommodityCode   string  `json:"cmdCode"`
	CommodityDesc   string  `json:"cmdDesc"`
	PrimaryValue    float64 `json:"primaryValue"` // Trade value in USD
	Period          string  `json:"period"`       // Year
}

type ComtradeResponse struct {
	Data []TradeFlow `json:"data"`
	Count int        `json:"count"`
}

// GetBilateralTrade fetches real trade data between two countries
// countryCode1: ISO3 code (e.g., "USA", "IND", "ARE")
// countryCode2: Partner country ISO3 code
// year: Trade year (e.g., "2023")
func (c *ComtradeClient) GetBilateralTrade(countryCode1, countryCode2, year string) ([]TradeFlow, error) {
	// Build API URL
	params := url.Values{}
	params.Add("reporterCode", countryCode1)
	params.Add("partnerCode", countryCode2)
	params.Add("period", year)
	params.Add("flowCode", "X") // Exports
	params.Add("frequency", "A") // Annual

	apiURL := fmt.Sprintf("%s/get?%s", c.BaseURL, params.Encode())

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "MargrafFDKG/1.0")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("comtrade API request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("comtrade API error %d: %s", resp.StatusCode, string(body))
	}

	var result ComtradeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse comtrade response: %v", err)
	}

	return result.Data, nil
}

// GetTopExports returns the top exported commodities from a country
func (c *ComtradeClient) GetTopExports(countryCode string, year string, limit int) ([]TradeFlow, error) {
	params := url.Values{}
	params.Add("reporterCode", countryCode)
	params.Add("partnerCode", "0") // World (all partners)
	params.Add("period", year)
	params.Add("flowCode", "X")
	params.Add("frequency", "A")

	apiURL := fmt.Sprintf("%s/get?%s", c.BaseURL, params.Encode())

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "MargrafFDKG/1.0")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("comtrade API error %d: %s", resp.StatusCode, string(body))
	}

	var result ComtradeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Sort by value and return top N
	flows := result.Data
	if len(flows) > limit {
		// Simple sort by value (descending)
		for i := 0; i < limit && i < len(flows); i++ {
			for j := i + 1; j < len(flows); j++ {
				if flows[j].PrimaryValue > flows[i].PrimaryValue {
					flows[i], flows[j] = flows[j], flows[i]
				}
			}
		}
		flows = flows[:limit]
	}

	return flows, nil
}

// CountryCodeMap maps common country names to ISO3 codes
var CountryCodeMap = map[string]string{
	"united states": "USA",
	"usa": "USA",
	"china": "CHN",
	"india": "IND",
	"united arab emirates": "ARE",
	"uae": "ARE",
	"japan": "JPN",
	"germany": "DEU",
	"united kingdom": "GBR",
	"uk": "GBR",
	"france": "FRA",
	"brazil": "BRA",
	"canada": "CAN",
	"russia": "RUS",
	"australia": "AUS",
	"south korea": "KOR",
	"mexico": "MEX",
	"indonesia": "IDN",
	"saudi arabia": "SAU",
	"turkey": "TUR",
	"switzerland": "CHE",
	"pakistan": "PAK",
	"vietnam": "VNM",
	"thailand": "THA",
}

// GetCountryCode returns ISO3 code for a country name
func GetCountryCode(countryName string) (string, bool) {
	code, ok := CountryCodeMap[countryName]
	return code, ok
}
