Cyndi Operator
==============

OpenShift operator that manages [Cyndi Pipelines](https://internal.cloud.redhat.com/docs/services/host-inventory/#host-data-syndication-aka-project-cyndi), i.e. data syndication between [Host-based Inventory](https://platform-docs.cloud.paas.psi.redhat.com/backend/inventory.html) and application databases.

A syndication pipeline consumes events from the [Inventory Event Interface](https://internal.cloud.redhat.com/docs/services/host-inventory/#event-interface) and materializes them in the target database.
[Kafka Connect](https://docs.confluent.io/current/connect/index.html) is used for consuming of the events stream.
[Custom transformations](https://github.com/redhatinsights/connect-transforms) are used to process the data.
It is then written into the database using using [JDBC Sink Connector](https://docs.confluent.io/3.1.1/connect/connect-jdbc/docs/sink_connector.html).

For more details about Host Data Syndication (a.k.a. Project Cyndi) see [platform documentation](https://internal.cloud.redhat.com/docs/services/host-inventory/#host-data-syndication-aka-project-cyndi)

![Architecture](./docs/architecture.png "Cyndi Architecture")

The operator is responsible for:

* management of database tables and views used to store the data in the target database
* management of connectors in a Kafka Connect cluster (using [Strimzi](https://strimzi.io/))
* periodic validation of the syndicated data
* automated recovery (e.g. when the data becomes out-of sync)

## Design

A Cyndi pipeline is represented by a custom resource of type [CyndiPipeline](https://github.com/RedHatInsights/cyndi-operator/blob/master/config/crd/bases/cyndi.cloud.redhat.com_cyndipipelines.yaml).

Cyndi operator is a cluster-scoped operator that reconciles `CyndiPipeline` resources.

The following diagram depicts the possible states of a Cyndi pipeline:

![State diagram](./docs/state-diagram.png "Pipeline State Diagram")

Typical flow

1. Once a *New* pipeline is created the operator creates a new database table and sets up a Kafka Connect connector that writes data into this table.
1. In the *Initial Sync* phase the connector seeds the database table by reading the inventory events topic.

   The topic is set up with enough retention to contain at least one event for each host.
   It is therefore at any time possible to re-create the target table from scratch.

1. Eventually, the target table becomes fully seeded and the pipeline transitions to *Valid* state.
   In this state the pipeline continues to be periodically validated and may transition to *Invalid* if data becomes out of sync.

1. If

   * the pipeline is *Invalid*, or
   * pipeline configuration changes, or
   * other problem is detected (e.g. database table is missing)

   then the pipeline is refreshed (transitions back to *New*).
   The old table and connector are kept while the new table is being seeded.
   Once the new table becomes *Valid* the `inventory.hosts` view is updated and the old table/connector are removed.

## Requirements

* [Strimzi-managed](https://strimzi.io/docs/operators/latest/quickstart.html) Kafka Connect cluster is running in the OpenShift cluster in the same namespace you intend to create `CyndiPipeline` resources in.
* A PostgreSQL database to be used as the target database
  * [Onboarding process](https://internal.cloud.redhat.com/docs/services/host-inventory/#onboarding-process) has been completed on the target database
  An OpenShift secret with database credentials is stored in the Kafka Connect namespace and named `{appName}-db`, where `appName` is the name used in pipeline definition. If needed, the name of the secret used can be changed by setting `dbSecret` in the `CyndiPipeline` spec.
* An OpenShift secret named `host-inventory-db` containing Inventory database credentials (used for validation) is present in the Kafka Connect namespace. The name of the secret used can be changed by setting `inventory.dbSecret` in the cyndi `ConfigMap`, or by setting `inventoryDbSecret` in the `CyndiPipeline` spec.


## Implementation

The operator defines two controllers that reconcile a Cyndi Pipeline
* [PipelineController](./controllers/cyndipipeline_controller.go) which manages connectors, database objects and handles automated recovery
* [ValidationController](./controllers/validation_controller.go) which periodically compares the data in the target database with what is stored in Host-based inventory to determine whether the pipeline is valid

### Reconcile loop

These are high level descriptions of the steps taken within the reconcile loop in each state:

* New

  1. Update the Pipeline's status with the pipeline version (uses the current timestamp)
  1. Create a new table in AppDB (e.g. inventory.hosts_v1_1597073300783716678)
  1. Create a new Kafka Sink Connector pointing to the new table

* Initial sync
  * Attempts to validate that the data is in sync.
    Each time the validation fails, it will requeue the reconcile loop until validation succeeds, or the retry limit is reached.
  * If the retry limit is reached before validation succeeds, the pipeline is refreshed (transitions to *New* state)
  * After validation succeeds, the DB view (inventory.hosts) is updated to point to the new table and the pipeline transitions to *Valid* state.

* Valid
  * ValidationController periodically validates the syndicated data. If data validation fails, the pipeline transitions to *Invalid* state

* Invalid
  * In the *Invalid* state the ValidationController continues with periodic validation of syndicated data.
  * If the validation succeeds, the pipeline transitions back to the *Valid* state.
  * If the pipeline fails to become valid before the retry limit is reached, the pipeline is refreshed (transitions to *New* state)

In addition, regardless of the state, PipelineController

* checks that configuration of the CyndiPipeline resource or the `cyndi` ConfigMap hasn't changed
* checks that the database table exists
* checks that the connector exists
* removes any stale database tables and connectors

### Validation

ValidationController currently only validates host identifiers.
It validates that all host identifiers stored in the Inventory database are present in the target database and that no other identifiers are stored in the target database.
The controller does not currently validate other fields (tags, system_profile, ...).

A threshold can be configured using the [cyndi ConfigMap](./examples/cyndi.configmap.yml).
The threshold causes the validation to pass as long as the ratio of invalid records is below this threshold (e.g. 1%)

## Development


### <a name='devenv'></a> Setting up the development environment

[CodeReady Containers](https://developers.redhat.com/products/codeready-containers/overview) can be used as the Kubernetes cluster.
Note that it requires a lot of RAM (over 20 GB on my machine).
[MiniKube](https://github.com/kubernetes/minikube/releases) is a less resource hungry option.
The rest of this document assumes CodeReady Containers.

1. Download an unpack [CodeReady Containers](https://developers.redhat.com/products/codeready-containers/overview)

1. Append the following line into `/etc/hosts`
    ```
    127.0.0.1 advisor-db inventory-db
    ```

1. Configure CRC to use 16G of memory
    ```
    ./crc config set memory 16384
    ```

1. Start CRC
    ```
    ./crc start
    ```

1. When prompted for a pull secret paste it (you obtained pull secret on step 1 when downloading CRC)

1. Log in to the cluster as kubeadmin (oc login -u kubeadmin -p ...)
   You'll find the exact command to use in the CRC startup log

1. Create a `cyndi` namespace
    ```
    oc create ns cyndi
    ```

1. Log in to https://quay.io/
   From Account settings download a kubernetes secret.
   This secret is used to pull quay.io/cloudservices images

1. Install the quay secret to the cluster
    ```
    oc apply -n cyndi -f <secret name>.yml
    ```

1. In the `dev` folder run
    ```
    ./run-admin.sh <secret name>
    ```
   and wait for it to finish

1. Set up port-forwarding to database pods
    ```
    oc port-forward svc/inventory-db 5432:5432 -n cyndi &
    oc port-forward svc/advisor-db 5433:5432 -n cyndi &
    ```

### Running the operator locally

With the cluster set up it is now possible to install manifests and run the operator locally.

1. Install CRDs
    ```
    make install
    ```

1. Run the operator
    ```
    make run ENABLE_WEBHOOKS=false
    ```

1. Finally, create a new pipeline
    ```
    oc apply -f ../config/samples/example-pipeline.yaml
    ```

    Optionally, you can wait for the pipeline to become valid with
    ```
    oc wait cyndi/example-pipeline --for=condition=Valid --timeout=300s -n cyndi
    ```

### Running the operator using OLM

An alternative to running the operator locally is to install the operator to the cluster using OLM.

1. Install OLM
    ```
    kubectl apply -f https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.17.0/crds.yaml
    kubectl apply -f https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.17.0/olm.yaml
    ```

1. Install the operator
    ```
    oc process -f ../deploy/operator.yml -p TARGET_NAMESPACE=cyndi -o yaml | oc apply -f -
    ```


### Development

[This is general info on building and running the operator.](https://master.sdk.operatorframework.io/docs/building-operators/golang/quickstart/)

As mentioned in the quickstart, run `make generate` any time a change is made to `cyndipipeline_types.go`.
Similarly, run `make manifests` any time a change is made which needs to regenerate the CRD manifests.
Finally, run `make install` to install/update the CRD in the Kubernetes cluster.

I find it easiest to run the operator locally.
This allows the use of a debugger.
Use `make delve` to start the operator in debug mode.
Then connect to it with a debugger on port 2345.
It can also be run locally with `make run ENABLE_WEBHOOKS=false`.

After everything is running, create a new Custom Resource via `kubectl apply -f config/samples/example-pipeline.yaml`.
Then, the CR can be managed via Kubernetes commands like normal.

### Running the tests

1. [Setup the dev environment](#devenv)
2. Forward ports via `dev/forward-ports.sh`
3. Add a test database to the HBI DB `create database test with template insights;`
4. Run the tests with `make test`

### Useful commands

Create a host
```
KAFKA_BOOTSTRAP_SERVERS=192.168.130.11:$(oc get service my-cluster-kafka-nodeport-0 -n cyndi -o=jsonpath='{.spec.ports[0].nodePort}{"\n"}') python utils/kafka_producer.py
```

List hosts
```
curl -H 'x-rh-identity: eyJpZGVudGl0eSI6eyJhY2NvdW50X251bWJlciI6IjAwMDAwMDEiLCAidHlwZSI6IlVzZXIifX0K' http://api.crc.testing:$(oc get service insights-inventory-public -n cyndi -o=jsonpath='{.spec.ports[0].nodePort}{"\n"}')/api/inventory/v1/hosts
```

Inspect Kafka Connect cluster
```
oc port-forward svc/my-connect-cluster-connect-api 8083:8083 -n cyndi
```
then access the kafka connect API at http://localhost:8083/connectors

Connect to inventory db
```
pgcli -h localhost -p 5432 -u insights insights
```

Connect to advisor db
```
pgcli -h localhost -p 5433 -u insights insights
```

Inspect index image
```
opm index export --index=quay.io/cloudservices/cyndi-operator-index:local -c podman
```
