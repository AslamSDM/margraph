package datasources

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WorldBankClient interfaces with World Bank API for economic indicators
// Documentation: https://datahelpdesk.worldbank.org/knowledgebase/articles/889392
type WorldBankClient struct {
	BaseURL string
	Client  *http.Client
}

func NewWorldBankClient() *WorldBankClient {
	return &WorldBankClient{
		BaseURL: "https://api.worldbank.org/v2",
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IndicatorValue represents an economic indicator value
type IndicatorValue struct {
	Indicator struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	} `json:"indicator"`
	Country struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	} `json:"country"`
	CountryISO3 string  `json:"countryiso3code"`
	Date        string  `json:"date"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Decimal     int     `json:"decimal"`
}

type WorldBankResponse struct {
	Page     int              `json:"page"`
	Pages    int              `json:"pages"`
	PerPage  int              `json:"per_page"`
	Total    int              `json:"total"`
	Data     []IndicatorValue `json:"-"`
}

// GetGDP fetches GDP data for a country
// Indicator: NY.GDP.MKTP.CD (GDP current USD)
func (w *WorldBankClient) GetGDP(countryCode string, year string) (*IndicatorValue, error) {
	return w.getIndicator(countryCode, "NY.GDP.MKTP.CD", year)
}

// GetFDI fetches Foreign Direct Investment data
// Indicator: BX.KLT.DINV.CD.WD (FDI net inflows)
func (w *WorldBankClient) GetFDI(countryCode string, year string) (*IndicatorValue, error) {
	return w.getIndicator(countryCode, "BX.KLT.DINV.CD.WD", year)
}

// GetTradeBalance fetches trade balance
// Indicator: NE.RSB.GNFS.CD (External balance on goods and services)
func (w *WorldBankClient) GetTradeBalance(countryCode string, year string) (*IndicatorValue, error) {
	return w.getIndicator(countryCode, "NE.RSB.GNFS.CD", year)
}

// GetExports fetches total exports
// Indicator: NE.EXP.GNFS.CD (Exports of goods and services)
func (w *WorldBankClient) GetExports(countryCode string, year string) (*IndicatorValue, error) {
	return w.getIndicator(countryCode, "NE.EXP.GNFS.CD", year)
}

// GetImports fetches total imports
// Indicator: NE.IMP.GNFS.CD (Imports of goods and services)
func (w *WorldBankClient) GetImports(countryCode string, year string) (*IndicatorValue, error) {
	return w.getIndicator(countryCode, "NE.IMP.GNFS.CD", year)
}

// getIndicator is a generic method to fetch any indicator
func (w *WorldBankClient) getIndicator(countryCode, indicatorCode, year string) (*IndicatorValue, error) {
	url := fmt.Sprintf("%s/country/%s/indicator/%s?date=%s&format=json",
		w.BaseURL, countryCode, indicatorCode, year)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "MargrafFDKG/1.0")

	resp, err := w.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("world bank API request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("world bank API error %d: %s", resp.StatusCode, string(body))
	}

	// World Bank API returns [metadata, data]
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response []json.RawMessage
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse world bank response: %v", err)
	}

	if len(response) < 2 {
		return nil, fmt.Errorf("no data available for %s in %s", countryCode, year)
	}

	var data []IndicatorValue
	if err := json.Unmarshal(response[1], &data); err != nil {
		return nil, fmt.Errorf("failed to parse indicator data: %v", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("no data points found")
	}

	return &data[0], nil
}

// GetEconomicProfile fetches comprehensive economic data for a country
type EconomicProfile struct {
	CountryCode  string
	CountryName  string
	Year         string
	GDP          float64
	FDI          float64
	Exports      float64
	Imports      float64
	TradeBalance float64
}

func (w *WorldBankClient) GetEconomicProfile(countryCode, year string) (*EconomicProfile, error) {
	profile := &EconomicProfile{
		CountryCode: countryCode,
		Year:        year,
	}

	// Fetch GDP
	if gdp, err := w.GetGDP(countryCode, year); err == nil {
		profile.GDP = gdp.Value
		profile.CountryName = gdp.Country.Value
	}

	// Fetch FDI
	if fdi, err := w.GetFDI(countryCode, year); err == nil {
		profile.FDI = fdi.Value
	}

	// Fetch Exports
	if exports, err := w.GetExports(countryCode, year); err == nil {
		profile.Exports = exports.Value
	}

	// Fetch Imports
	if imports, err := w.GetImports(countryCode, year); err == nil {
		profile.Imports = imports.Value
	}

	// Fetch Trade Balance
	if balance, err := w.GetTradeBalance(countryCode, year); err == nil {
		profile.TradeBalance = balance.Value
	}

	return profile, nil
}

// GetTopTradingPartners estimates top trading partners based on trade volume
// Note: World Bank doesn't provide bilateral trade, so this is estimated from total trade
func (w *WorldBankClient) GetTradeIntensity(countryCode string, year string) (float64, error) {
	exports, err := w.GetExports(countryCode, year)
	if err != nil {
		return 0, err
	}

	gdp, err := w.GetGDP(countryCode, year)
	if err != nil {
		return 0, err
	}

	// Trade intensity = (Exports / GDP) * 100
	if gdp.Value == 0 {
		return 0, fmt.Errorf("GDP is zero")
	}

	intensity := (exports.Value / gdp.Value) * 100
	return intensity, nil
}
