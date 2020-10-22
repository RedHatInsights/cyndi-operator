package v1beta1

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type PipelineState string

const (
	STATE_NEW          PipelineState = "NEW"
	STATE_INITIAL_SYNC PipelineState = "INITIAL_SYNC"
	STATE_VALID        PipelineState = "VALID"
	STATE_INVALID      PipelineState = "INVALID"
	STATE_REMOVED      PipelineState = "REMOVED"
	STATE_UNKNOWN      PipelineState = "UNKNOWN"
)

const tablePrefix = "hosts_v"

func (instance *CyndiPipeline) GetState() PipelineState {
	switch {
	case instance.GetDeletionTimestamp() != nil:
		return STATE_REMOVED
	case instance.Status.PipelineVersion == "":
		return STATE_NEW
	case instance.Status.InitialSyncInProgress == true:
		return STATE_INITIAL_SYNC
	case instance.Status.InitialSyncInProgress == false && instance.Status.SyndicatedDataIsValid == true:
		return STATE_VALID
	case instance.Status.InitialSyncInProgress == false && instance.Status.SyndicatedDataIsValid == false:
		return STATE_INVALID
	default:
		return STATE_UNKNOWN
	}
}

func (instance *CyndiPipeline) TransitionTo(state PipelineState) error {

	switch state {
	case STATE_INITIAL_SYNC:
		if instance.GetState() != STATE_NEW {
			return invalidTransition(instance.GetState(), state)
		}

		instance.Status.InitialSyncInProgress = true
		instance.Status.SyndicatedDataIsValid = false
		instance.Status.PipelineVersion = fmt.Sprintf("1_%s", strconv.FormatInt(time.Now().UnixNano(), 10))
		instance.Status.ConnectorName = ConnectorName(instance.Status.PipelineVersion, instance.Spec.AppName)
		instance.Status.TableName = TableName(instance.Status.PipelineVersion)
	}

	return nil
}

func invalidTransition(from PipelineState, to PipelineState) error {
	return fmt.Errorf("Attempted invalid state transition from %s to %s", from, to)
}

func TableName(pipelineVersion string) string {
	return fmt.Sprintf("%s%s", tablePrefix, pipelineVersion)
}

func TableNameToConnectorName(tableName string, appName string) string {
	return ConnectorName(string(tableName[len(tablePrefix):len(tableName)]), appName)
}

func ConnectorName(pipelineVersion string, appName string) string {
	return fmt.Sprintf("syndication-pipeline-%s-%s", appName, strings.Replace(pipelineVersion, "_", "-", 1))
}
