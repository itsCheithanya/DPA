package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
)

const (
	historyLength   = 10
	aiModelEndpoint = "http://192.168.1.5:32249/predict"
)

var (
	rpsData            = make([]float64, historyLength)
	rpsDataFromScraper = make([]float64, historyLength)
)

func Predicter(scrapeURL string) (float64, float64, error) {
	// Query current RPS
	currentVal, err := scrapeRPS(scrapeURL)
	if err != nil {
		fmt.Println("scrape fetch error:", err)
		return 0.0, 0.0, err
	}

	// Track delta RPS
	if rpsDataFromScraper[historyLength-1] != 0 {
		delta := currentVal - rpsDataFromScraper[historyLength-1]
		rpsData = append(rpsData[1:], delta)
		rpsDataFromScraper = append(rpsDataFromScraper[1:], currentVal)
	} else {
		rpsDataFromScraper = append(rpsDataFromScraper[1:], currentVal)
	}

	// Normalize input
	normInput := normalize(rpsData)

	// Prepare request payload
	payload := map[string][]float64{
		"input": normInput,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return currentVal, 0.0, err
	}

	// Send POST request to the model service
	resp, err := http.Post(aiModelEndpoint, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return currentVal, 0.0, err
	}
	defer resp.Body.Close()

	// Parse response
	var result struct {
		Prediction [][]float64 `json:"prediction"`
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return currentVal, 0.0, err
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return currentVal, 0.0, err
	}

	predictedNorm := result.Prediction[0][0]
	predictedWorkload := int(math.Ceil(float64(predictedNorm)*(max(rpsData)-min(rpsData)) + min(rpsData)))

	fmt.Println("Predicted Workload RPS:", predictedWorkload)

	return float64(rpsData[len(rpsData)-1]), float64(predictedWorkload), nil
}

func scrapeRPS(url string) (float64, error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Result []struct {
				Value []interface{} `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, err
	}

	if len(result.Data.Result) == 0 {
		return 0, fmt.Errorf("no data in Prometheus response")
	}

	valStr := result.Data.Result[0].Value[1].(string)
	var curVal float64
	fmt.Sscanf(valStr, "%f", &curVal)

	return curVal, nil
}

func normalize(input []float64) []float64 {
	minVal := min(input)
	maxVal := max(input)

	scaled := make([]float64, len(input))
	if maxVal == minVal {
		// Avoid divide by zero; return 0s
		for i := range input {
			scaled[i] = 0
		}
		return scaled
	}

	for i, v := range input {
		scaled[i] = (v - minVal) / (maxVal - minVal)
	}
	return scaled
}

func min(slice []float64) float64 {
	minVal := slice[0]
	for _, v := range slice {
		if v < minVal {
			minVal = v
		}
	}
	return minVal
}

func max(slice []float64) float64 {
	maxVal := slice[0]
	for _, v := range slice {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

// package controller

// import (
// 	// "bytes"
// 	"encoding/json"
// 	"fmt"
// 	"io/ioutil"
// 	"math"
// 	"net/http"
// 	// "time"

// 	tf "github.com/tensorflow/tensorflow/tensorflow/go"
// 	// "gonum.org/v1/gonum/mat"
// )

// const (
// 	// modelPath      = "./saved_model" // path to exported .pb model
// 	modelPath = "./internal/controller/saved_models"
// 	// prometheusURL  = "http://localhost:9091/api/v1/query?query=node_request_operations_total"
// 	historyLength = 10
// )

// var (
// 	rpsData            = make([]float64, historyLength)
// 	rpsDataFromScraper = make([]float64, historyLength)
// )

// func Predicter(scrapeURL string) (float64, float64) {
// 	// Load TensorFlow model
// 	model, err := tf.LoadSavedModel(modelPath, []string{"serve"}, nil)
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer model.Session.Close()

// 	// Query ScrapeURL for current workload (in requests per second)
// 	currentVal, err := scrapeRPS(scrapeURL)
// 	if err != nil {
// 		fmt.Println("scrape fetch error:", err)
// 	}

// 	// Track delta RPS
// 	if rpsDataFromScraper[historyLength-1] != 0 {
// 		// the data from the scraper should be a incremental data, example: T[rps]=x,T[rps+1]=x+currentRPS
// 		delta := currentVal - rpsDataFromScraper[historyLength-1]
// 		rpsData = append(rpsData[1:], delta)
// 		rpsDataFromScraper = append(rpsDataFromScraper[1:], currentVal)
// 	} else {
// 		rpsDataFromScraper = append(rpsDataFromScraper[1:], currentVal)
// 	}

// 	// Normalize input
// 	normInput := normalize(rpsData)

// 	// Create input tensor
// 	tensor, err := tf.NewTensor([][][]float32{{}}) // Shape: [1,10,1]
// 	if err != nil {
// 		panic(err)
// 	}
// 	for _, val := range normInput {
// 		tensor.Value().([][][]float32)[0] = append(tensor.Value().([][][]float32)[0], []float32{float32(val)})
// 	}

// 	// Run prediction
// 	output, err := model.Session.Run(
// 		map[tf.Output]*tf.Tensor{
// 			model.Graph.Operation("serving_default_input_1").Output(0): tensor,
// 		},
// 		[]tf.Output{
// 			model.Graph.Operation("StatefulPartitionedCall").Output(0),
// 		},
// 		nil,
// 	)
// 	if err != nil {
// 		panic(err)
// 	}

// 	// Postprocess: inverse normalize
// 	predictedNorm := output[0].Value().([][]float32)[0][0]
// 	predictedWorkload := int(math.Ceil(float64(predictedNorm) * (max(rpsData) - min(rpsData) + min(rpsData))))

// 	fmt.Println("Predicted Workload RPS:", predictedWorkload)

// 	return float64(rpsData[len(rpsData)-1]), float64(predictedWorkload)
// }
