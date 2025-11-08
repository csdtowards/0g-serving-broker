package ctrl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/0glabs/0g-serving-broker/common/errors"
	"github.com/0glabs/0g-serving-broker/common/util"
	"github.com/0glabs/0g-serving-broker/inference/model"
)

// GetTextToImageInputFeeAndSteps gets input fee and steps for text-to-image generation
func (c *Ctrl) GetTextToImageInputFeeAndSteps(reqBody []byte) (string, int64, error) {
	var request map[string]interface{}
	if err := json.Unmarshal(reqBody, &request); err != nil {
		return "", 0, errors.Wrap(err, "failed to unmarshal request body")
	}
	
	// Get steps parameter (prefer "steps", fallback to "num_inference_steps")
	var steps int64
	if stepVal, exists := request["steps"]; exists {
		if stepFloat, ok := stepVal.(float64); ok {
			steps = int64(stepFloat)
		}
	} else if numStepsVal, exists := request["num_inference_steps"]; exists {
		if stepFloat, ok := numStepsVal.(float64); ok {
			steps = int64(stepFloat)
		}
	}
	
	// Use default steps if not specified
	if steps == 0 {
		steps = 50 // default steps
	}
	
	// Input fee is fixed at 0 (like zgStorage)
	expectedInputFee := "0"
	
	return expectedInputFee, steps, nil
}

// handleTextToImageResponse handles image generation response
func (c *Ctrl) handleTextToImageResponse(ctx *gin.Context, resp *http.Response, account model.User, outputPrice int64, reqBody []byte, reqModel model.Request) error {
	defer resp.Body.Close()
	
	// Read and return image data
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.handleBrokerError(ctx, err, "read image response body")
		return err
	}
	
	// Return image to client
	if _, err := ctx.Writer.Write(body); err != nil {
		c.handleBrokerError(ctx, err, "write image response")
		return err
	}
	
	// Get steps from request for billing
	steps := reqModel.OutputCount // previously stored steps count
	
	// Calculate output fee: steps × price per step
	outputFee, err := util.Multiply(outputPrice, steps)
	if err != nil {
		return errors.Wrap(err, "calculate output fee based on steps")
	}
	
	// Update account and request records
	return c.updateAccountWithImageGeneration(ctx, outputFee.String(), reqModel.RequestHash, steps)
}

// updateAccountWithImageGeneration updates fee records for image generation
func (c *Ctrl) updateAccountWithImageGeneration(ctx context.Context, outputFee string, requestHash string, steps int64) error {
	request, err := c.db.GetRequest(requestHash)
	if err != nil {
		return errors.Wrap(err, "get request from database")
	}
	
	// Calculate total fee: input fee + output fee (based on steps)
	totalFee, err := util.Add(request.InputFee, outputFee)
	if err != nil {
		return errors.Wrap(err, "calculate total fee")
	}
	
	// Update database: output fee, total fee, output count (steps)
	if err := c.db.UpdateRequestFeesAndCount(requestHash, outputFee, totalFee.String(), steps); err != nil {
		return errors.Wrap(err, "update request fees and count in database")
	}
	
	return nil
}