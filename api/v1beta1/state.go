package v1beta1

import (
	"fmt"
	"strings"
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
	case instance.Status.SyndicatedDataIsValid == true:
		return STATE_VALID
	case instance.Status.InitialSyncInProgress == true:
		return STATE_INITIAL_SYNC
	case instance.Status.SyndicatedDataIsValid == false && instance.Status.InitialSyncInProgress == false:
		return STATE_INVALID
	default:
		return STATE_UNKNOWN
	}
}

func (instance *CyndiPipeline) TransitionToNew() error {
	instance.Status.InitialSyncInProgress = false
	instance.Status.SyndicatedDataIsValid = false
	instance.Status.ValidationFailedCount = 0
	instance.Status.PipelineVersion = ""
	return nil
}

func (instance *CyndiPipeline) TransitionToInitialSync(pipelineVersion string) error {
	if err := instance.assertState(STATE_INITIAL_SYNC, STATE_INITIAL_SYNC, STATE_NEW); err != nil {
		return err
	}

	instance.Status.InitialSyncInProgress = true
	instance.Status.SyndicatedDataIsValid = false
	instance.Status.ValidationFailedCount = 0
	instance.Status.PipelineVersion = pipelineVersion
	instance.Status.ConnectorName = ConnectorName(pipelineVersion, instance.Spec.AppName)
	instance.Status.TableName = TableName(pipelineVersion)

	return nil
}

func (instance *CyndiPipeline) TransitionToValid() error {
	if err := instance.assertState(STATE_VALID, STATE_INITIAL_SYNC, STATE_VALID, STATE_INVALID); err != nil {
		return err
	}

	instance.Status.SyndicatedDataIsValid = true
	instance.Status.PreviousPipelineVersion = ""
	instance.Status.InitialSyncInProgress = false

	return nil
}

func (instance *CyndiPipeline) assertState(targetState PipelineState, validStates ...PipelineState) error {
	for _, state := range validStates {
		if instance.GetState() == state {
			return nil
		}
	}

	return fmt.Errorf("Attempted invalid state transition from %s to %s", instance.GetState(), targetState)
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
