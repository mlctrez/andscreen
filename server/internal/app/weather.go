package app

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/briandowns/openweathermap"
	"github.com/joho/godotenv"
)

type dayReading struct {
	name      string
	high      string
	low       string
	condition string
}

// weatherView is what the screen draws. Temps stay empty until a fetch succeeds.
type weatherView struct {
	ready     bool
	current   string
	condition string
	todayHigh string
	todayLow  string
	days      []dayReading
}

// loadEnv reads a .env file in the working directory for development.
// Existing environment variables are left as they are, which is how a service
// supplies OWM_KEY, OWM_ZIP, and LISTEN. A missing .env file is not an error.
func loadEnv() error {
	err := godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// weatherAllowed is 6:00 AM through 11:00 PM local time, inclusive.
func weatherAllowed(t time.Time) bool {
	mins := t.Hour()*60 + t.Minute()
	return mins >= 6*60 && mins <= 23*60
}

func nextHalfHour(t time.Time) time.Time {
	return t.Truncate(30 * time.Minute).Add(30 * time.Minute)
}

func fetchWeather(now time.Time) (weatherView, error) {
	key := os.Getenv("OWM_KEY")
	zip := os.Getenv("OWM_ZIP")
	if key == "" || zip == "" {
		return weatherView{}, fmt.Errorf("OWM_KEY and OWM_ZIP are not set")
	}
	cur, err := openweathermap.NewCurrent("F", "en", key)
	if err != nil {
		return weatherView{}, err
	}
	if err = cur.CurrentByZipcode(zip, "US"); err != nil {
		return weatherView{}, err
	}
	fc, err := openweathermap.NewForecast("5", "F", "en", key)
	if err != nil {
		return weatherView{}, err
	}
	if err = fc.DailyByZipcode(zip, "US", 40); err != nil {
		return weatherView{}, err
	}
	data, ok := fc.ForecastWeatherJson.(*openweathermap.Forecast5WeatherData)
	if !ok || data == nil {
		return weatherView{}, fmt.Errorf("forecast response had no 5-day data")
	}
	condition := ""
	if len(cur.Weather) > 0 {
		condition = cur.Weather[0].Main
	}
	return summarizeWeather(now, cur.Main.Temp, condition, data.List), nil
}

func summarizeWeather(now time.Time, current float64, condition string, list []openweathermap.Forecast5WeatherList) weatherView {
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	high, low := current, current
	type acc struct {
		when      time.Time
		high, low float64
		condition string
		condDist  int
	}
	var days []acc
	for _, item := range list {
		when := time.Unix(int64(item.Dt), 0).In(loc)
		day := time.Date(when.Year(), when.Month(), when.Day(), 0, 0, 0, 0, loc)
		lo, hi := item.Main.TempMin, item.Main.TempMax
		if day.Equal(today) {
			if hi > high {
				high = hi
			}
			if lo < low {
				low = lo
			}
			continue
		}
		if day.Before(today) {
			continue
		}
		cond, dist := forecastCondition(item, when)
		if n := len(days); n > 0 && days[n-1].when.Equal(day) {
			if hi > days[n-1].high {
				days[n-1].high = hi
			}
			if lo < days[n-1].low {
				days[n-1].low = lo
			}
			if cond != "" && (days[n-1].condition == "" || dist < days[n-1].condDist) {
				days[n-1].condition = cond
				days[n-1].condDist = dist
			}
			continue
		}
		days = append(days, acc{when: day, high: hi, low: lo, condition: cond, condDist: dist})
	}
	view := weatherView{
		ready:     true,
		current:   formatTemp(current),
		condition: condition,
		todayHigh: formatTemp(high),
		todayLow:  formatTemp(low),
	}
	for _, day := range days {
		if len(view.days) == 5 {
			break
		}
		view.days = append(view.days, dayReading{
			name:      day.when.Format("Mon"),
			high:      formatTemp(day.high),
			low:       formatTemp(day.low),
			condition: day.condition,
		})
	}
	return view
}

func formatTemp(v float64) string {
	return fmt.Sprintf("%d°", int(math.Round(v)))
}

// forecastCondition returns the slot's condition and how far its hour is from
// mid-afternoon, so a day's label prefers the daytime reading.
func forecastCondition(item openweathermap.Forecast5WeatherList, when time.Time) (string, int) {
	if len(item.Weather) == 0 || item.Weather[0].Main == "" {
		return "", 24
	}
	dist := when.Hour() - 15
	if dist < 0 {
		dist = -dist
	}
	return item.Weather[0].Main, dist
}
