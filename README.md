[![Go Report Card][go-report-svg]][go-report-card]
[![Coverage][test-and-build-svg]][test-and-build]
[![Security][security-badge]][security-pipelines]
[![Coverage Status][coverage-badge]][coverage]
[![Documentations][website-badge]][website]
[![Adopters][adopters-badge]][adopters]
[![CNCF Landscape][cncf-badge]][cncf-landscape]

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/kube-green/kube-green/main/logo/logo-horizontal-dark.svg">
  <img alt="Dark kube-green logo" src="https://raw.githubusercontent.com/kube-green/kube-green/main/logo/logo-horizontal.svg">
</picture>

How many of your dev/preview pods stay on during weekends? Or at night? It's a waste of resources! And money! But fear not, *kube-green* is here to the rescue.

*kube-green* is a simple **k8s addon** that automatically **shuts down** (some of) your **resources** when you don't need them.

## 🚀 Extended Features (This Fork)

This fork extends kube-green with:

- ✅ **REST API** with JWT authentication
- ✅ **Role-Based Access Control** (admin, operacion, lectura)
- ✅ **Web Frontend** (React-based UI)
- ✅ **Extended CRD Support** (PgCluster, HDFSCluster, OsCluster, KafkaCluster, PgBouncer)
- ✅ **User Management** via admin panel
- ✅ **Dynamic Resource Detection** in namespaces
- ✅ **Staged Wake-Up** sequences for proper service dependencies
- ✅ **Multi-tenant Support** with helper scripts
- ✅ **Comprehensive API Documentation** (Swagger + HTML)

If you already use *kube-green*, add yourself as an [adopter][add-adopters]!

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes. See how to install the project on a live system in our [docs](https://kube-green.dev/docs/installation/).

### Prerequisites

Make sure you have Go installed ([download](https://go.dev/dl/)). Version 1.19 or higher is required.

## Installation

To have *kube-green* running locally just clone this repository and install the dependencies running:

```golang
go get
```

## Running the tests

There are different types of tests in this repository.

It is possible to run all the unit tests with

```sh
make test
```

To run integration tests installing kube-green with kustomize, run:

```sh
make e2e-test-kustomize
```

otherwise, to run integration tests installing kube-green with helm, run:

```sh
make e2e-test
```

It is possible to run only a specific harness integration test, running e2e-test with the OPTION variable:

```sh
make e2e-test OPTION="-run=TestSleepInfoE2E/kuttl/run_e2e_tests/harness/{TEST_NAME}"
```

## Deployment

To deploy *kube-green* in live systems, follow the [docs](https://kube-green.dev/docs/installation/).

To run kube-green for development purpose, you can use [ko](https://ko.build/) to deploy
in a KinD cluster.
It is possible to start a KinD cluster running `kind create cluster --name kube-green-development`.
To deploy kube-green using ko, run:

```sh
make local-run clusterName=kube-green-development
```

## Usage

The use of this operator is very simple. Once installed on the cluster, configure the desired CRD to make it works.

See [here](https://kube-green.dev/docs/configuration/) the documentation about the configuration of the CRD.

### CRD Examples

**Note:** The `timeZone` field uses IANA time zone identifiers. If not set, it defaults to UTC. You can set it to any valid timezone such as `Europe/Rome`, `America/New_York`, etc.

Pods running during working hours with Europe/Rome timezone, suspend CronJobs and exclude a deployment named `api-gateway`:

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: working-hours
spec:
  weekdays: "1-5"
  sleepAt: "20:00"
  wakeUpAt: "08:00"
  timeZone: "Europe/Rome"
  suspendCronJobs: true
  excludeRef:
    - apiVersion: "apps/v1"
      kind:       Deployment
      name:       api-gateway
```

Pods sleep every night without restore:

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: working-hours-no-wakeup
spec:
  sleepAt: "20:00"
  timeZone: Europe/Rome
  weekdays: "*"
```

### Extended CRD Examples

**Note:** The following examples show how to use the extended functionality for managing CRDs (PgCluster, HDFSCluster, OsCluster, KafkaCluster, PgBouncer) directly without the helper script.

**Example 1: Suspend PostgreSQL cluster using native CRD support**

This example suspends a PgCluster CRD by setting the shutdown annotation. The postgres-operator will handle the actual shutdown:

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: postgres-schedule
  namespace: my-namespace
spec:
  weekdays: "1-5"
  sleepAt: "20:00"
  wakeUpAt: "08:00"
  timeZone: "America/Bogota"
  suspendStatefulSetsPostgres: true
  excludeRef:
    - matchLabels:
        app.kubernetes.io/managed-by: postgres-operator
        postgres.stratio.com/cluster: "true"
```

**Example 2: Suspend HDFS cluster using native CRD support**

This example suspends an HDFSCluster CRD by setting the shutdown annotation:

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: hdfs-schedule
  namespace: my-namespace
spec:
  weekdays: "1-5"
  sleepAt: "20:00"
  wakeUpAt: "08:00"
  timeZone: "America/Bogota"
  suspendStatefulSetsHdfs: true
  excludeRef:
    - matchLabels:
        app.kubernetes.io/managed-by: hdfs-operator
        hdfs.stratio.com/cluster: "true"
```

**Example 3: Suspend PgBouncer instances using native CRD support**

This example suspends PgBouncer CRDs by modifying the `spec.instances` field:

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: pgbouncer-schedule
  namespace: my-namespace
spec:
  weekdays: "1-5"
  sleepAt: "20:00"
  wakeUpAt: "08:00"
  timeZone: "America/Bogota"
  suspendDeploymentsPgbouncer: true
```

**Example 4: Suspend OpenSearch cluster using native CRD support**

This example suspends an OsCluster CRD by setting the shutdown annotation. The opensearch-operator will handle the actual shutdown:

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: opensearch-schedule
  namespace: my-namespace
spec:
  weekdays: "1-5"
  sleepAt: "20:00"
  wakeUpAt: "08:00"
  timeZone: "America/Bogota"
  suspendStatefulSetsOpenSearch: true
  excludeRef:
    - matchLabels:
        app.kubernetes.io/managed-by: opensearch-operator
        opensearch.stratio.com/cluster: "true"
```

**Example 5: Suspend Kafka cluster using native CRD support**

This example suspends a KafkaCluster CRD by setting the shutdown annotation. The kafka-operator will handle the actual shutdown:

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: kafka-schedule
  namespace: my-namespace
spec:
  weekdays: "1-5"
  sleepAt: "20:00"
  wakeUpAt: "08:00"
  timeZone: "America/Bogota"
  suspendStatefulSetsKafka: true
  excludeRef:
    - matchLabels:
        app.kubernetes.io/managed-by: kafka-operator
        kafka.stratio.com/cluster: "true"
```

**Example 6: Staged wake-up for datastores namespace (Postgres, HDFS, OpenSearch, Kafka, PgBouncer, and native deployments)**

This example uses separate SleepInfos with shared annotations to implement staged wake-up. The `pair-id` and `pair-role` annotations allow the wake SleepInfos to find restore patches from the sleep SleepInfo:

**Sleep SleepInfo** (suspends all resources):

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: sleep-datastores
  namespace: my-datastores
  annotations:
    kube-green.stratio.com/pair-id: "my-datastores"
    kube-green.stratio.com/pair-role: "sleep"
spec:
  weekdays: "1-5"
  sleepAt: "20:00"
  timeZone: "America/Bogota"
  suspendDeployments: true
  suspendStatefulSets: true
  suspendCronJobs: true
  suspendDeploymentsPgbouncer: true
  suspendStatefulSetsPostgres: true
  suspendStatefulSetsHdfs: true
  suspendStatefulSetsOpenSearch: true
  suspendStatefulSetsKafka: true
  excludeRef:
    - matchLabels:
        app.kubernetes.io/managed-by: postgres-operator
    - matchLabels:
        postgres.stratio.com/cluster: "true"
    - matchLabels:
        app.kubernetes.io/managed-by: hdfs-operator
    - matchLabels:
        hdfs.stratio.com/cluster: "true"
    - matchLabels:
        app.kubernetes.io/managed-by: opensearch-operator
    - matchLabels:
        opensearch.stratio.com/cluster: "true"
    - matchLabels:
        app.kubernetes.io/managed-by: kafka-operator
    - matchLabels:
        kafka.stratio.com/cluster: "true"
```

**Wake SleepInfo for Postgres, HDFS, OpenSearch, and Kafka** (first stage - 5 minutes before others):

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: wake-datastores-pg-hdfs-opensearch-kafka
  namespace: my-datastores
  annotations:
    kube-green.stratio.com/pair-id: "my-datastores"
    kube-green.stratio.com/pair-role: "wake"
spec:
  weekdays: "1-5"
  sleepAt: "07:55"
  timeZone: "America/Bogota"
  suspendStatefulSetsPostgres: true
  suspendStatefulSetsHdfs: true
  suspendStatefulSetsOpenSearch: true
  suspendStatefulSetsKafka: true
  excludeRef:
    - matchLabels:
        app.kubernetes.io/managed-by: postgres-operator
    - matchLabels:
        postgres.stratio.com/cluster: "true"
    - matchLabels:
        app.kubernetes.io/managed-by: hdfs-operator
    - matchLabels:
        hdfs.stratio.com/cluster: "true"
    - matchLabels:
        app.kubernetes.io/managed-by: opensearch-operator
    - matchLabels:
        opensearch.stratio.com/cluster: "true"
    - matchLabels:
        app.kubernetes.io/managed-by: kafka-operator
    - matchLabels:
        kafka.stratio.com/cluster: "true"
```

**Wake SleepInfo for PgBouncer** (second stage - 2 minutes after Postgres/HDFS/OpenSearch/Kafka):

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: wake-datastores-pgbouncer
  namespace: my-datastores
  annotations:
    kube-green.stratio.com/pair-id: "my-datastores"
    kube-green.stratio.com/pair-role: "wake"
spec:
  weekdays: "1-5"
  sleepAt: "07:57"
  timeZone: "America/Bogota"
  suspendDeploymentsPgbouncer: true
```

**Wake SleepInfo for native deployments** (final stage - at wake time):

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: wake-datastores
  namespace: my-datastores
  annotations:
    kube-green.stratio.com/pair-id: "my-datastores"
    kube-green.stratio.com/pair-role: "wake"
spec:
  weekdays: "1-5"
  sleepAt: "08:00"
  timeZone: "America/Bogota"
  suspendDeployments: true
  suspendStatefulSets: true
  suspendCronJobs: true
  suspendDeploymentsPgbouncer: true
  excludeRef:
    - matchLabels:
        app.kubernetes.io/managed-by: postgres-operator
    - matchLabels:
        postgres.stratio.com/cluster: "true"
    - matchLabels:
        app.kubernetes.io/managed-by: hdfs-operator
    - matchLabels:
        hdfs.stratio.com/cluster: "true"
    - matchLabels:
        app.kubernetes.io/managed-by: opensearch-operator
    - matchLabels:
        opensearch.stratio.com/cluster: "true"
    - matchLabels:
        app.kubernetes.io/managed-by: kafka-operator
    - matchLabels:
        kafka.stratio.com/cluster: "true"
```

**Note:** The staged wake-up ensures services start in the correct order: Postgres/HDFS/OpenSearch/Kafka first (needed by all), then PgBouncer (depends on Postgres), and finally native deployments (depend on databases).

To see other examples, go to [our docs](https://kube-green.dev/docs/configuration/#examples).

## Extensions

This fork includes extended functionality for managing Custom Resource Definitions (CRDs), REST API with authentication, role-based access control, and a helper script for multi-tenant environments.

### Extended CRD Support

This fork extends kube-green with native support for managing these CRDs:

- **PgCluster**: PostgreSQL clusters managed by the postgres-operator
- **HDFSCluster**: HDFS clusters managed by the hdfs-operator
- **OsCluster**: OpenSearch clusters managed by the opensearch-operator
- **KafkaCluster**: Kafka clusters managed by the kafka-operator
- **PgBouncer**: PgBouncer instances managed by the postgres-operator

These CRDs are managed through annotation-based patches:
- PgCluster: `pgcluster.stratio.com/shutdown=true|false`
- HDFSCluster: `hdfscluster.stratio.com/shutdown=true|false`
- OsCluster: `oscluster.stratio.com/shutdown=true|false`
- KafkaCluster: `kafkacluster.stratio.com/shutdown=true|false`
- PgBouncer: `spec.instances` field (native support)

**Example: Managing OpenSearch and Kafka clusters**

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: sleep-opensearch-kafka
  namespace: my-namespace
spec:
  weekdays: "5"  # Friday
  sleepAt: "22:00"
  wakeUpAt: "06:00"
  timeZone: "UTC"
  suspendStatefulSetsOpenSearch: true  # Manages OsCluster
  suspendStatefulSetsKafka: true       # Manages KafkaCluster
```

**Example: Managing all CRDs together**

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: sleep-all-crds
  namespace: my-namespace
spec:
  weekdays: "5"
  sleepAt: "22:00"
  wakeUpAt: "06:00"
  timeZone: "UTC"
  suspendStatefulSetsPostgres: true
  suspendStatefulSetsHdfs: true
  suspendStatefulSetsOpenSearch: true
  suspendStatefulSetsKafka: true
  suspendDeploymentsPgbouncer: true
```

### Staged Wake-Up

The extended version supports staged wake-up sequences to ensure proper service dependencies:
1. Postgres, HDFS, OpenSearch, and Kafka clusters (needed for all services)
2. PgBouncer instances (5 minutes after, depends on Postgres)
3. Native Deployments/StatefulSets (7 minutes after, depend on databases)

The system automatically detects CRDs in namespaces and configures appropriate wake-up sequences with delays to ensure services start in the correct order.

### tenant_power.py Helper Script

For multi-tenant environments, use the `tenant_power.py` script to easily configure sleep/wake schedules. This script simplifies management by:

- Automatically converting local time (America/Bogota) to UTC
- Adjusting weekdays based on timezone conversion
- Creating SleepInfo configurations for all namespaces
- Applying staged wake-up sequences
- Managing Postgres, HDFS, PgBouncer, and native applications

#### Prerequisites

```bash
pip install ruamel.yaml
```

#### Quick Start Examples

**Example 1: Configure all services to sleep Monday-Friday at 10 PM and wake at 6 AM**

```bash
python3 tenant_power.py create --tenant bdadevprd --off 22:00 --on 06:00 \
    --weekdays "lunes-viernes" --apply
```

**Example 2: Configure only a specific namespace (airflowsso) to sleep Friday at 11 PM and wake Monday at 6 AM**

```bash
python3 tenant_power.py create --tenant bdadevprd --off 23:00 --on 06:00 \
    --sleepdays "viernes" --wakedays "lunes" --namespaces airflowsso --apply
```

**Example 3: Generate YAML file without applying (useful for review)**

```bash
python3 tenant_power.py create --tenant bdadevprd --off 22:00 --on 06:00 \
    --weekdays "lunes-viernes" --outdir ./yamls
```

**Example 4: View current configurations**

```bash
python3 tenant_power.py show --tenant bdadevprd
```

**Example 5: Update existing configurations**

```bash
python3 tenant_power.py update --tenant bdadevprd --off 23:00 --on 07:00 \
    --weekdays "lunes-viernes" --apply
```

#### Available Commands

- **create**: Create new sleep/wake configurations for a tenant
- **update**: Update existing configurations
- **show**: Display current configurations in a human-readable format

#### Command Options

- `--tenant`: Tenant name (required, e.g., `bdadevprd`, `bdadevdat`, `bdadevlab`)
- `--off`: Sleep time in local timezone (required, format `HH:MM`, e.g., `22:00`, `14:15`)
- `--on`: Wake time in local timezone (required, format `HH:MM`, e.g., `06:00`, `14:25`)
- `--weekdays`: Days of the week (default: all days). Can use human format (`"lunes-viernes"`, `"sábado"`) or numeric (`"1-5"`, `"6"`)
- `--sleepdays`: (Optional) Specific days for sleep. If not specified, uses `--weekdays`
- `--wakedays`: (Optional) Specific days for wake. If not specified, uses `--weekdays`
- `--namespaces`: (Optional) Limit to specific namespaces. Valid values: `datastores`, `apps`, `rocket`, `intelligence`, `airflowsso`
- `--apply`: Apply changes directly to Kubernetes cluster (without this, only generates YAML)
- `--outdir`: Directory to save generated YAML file (when not using `--apply`)

#### Supported Namespaces

The script manages these namespace types:
- **datastores**: Databases (Postgres, HDFS, PgBouncer)
- **apps**: Main applications
- **rocket**: Rocket services
- **intelligence**: Intelligence services
- **airflowsso**: Airflow SSO services

For more detailed usage, run:

```bash
python3 tenant_power.py --help
```

Or for specific command help:

```bash
python3 tenant_power.py create --help
python3 tenant_power.py update --help
python3 tenant_power.py show --help
```

## REST API

This fork includes a comprehensive REST API for managing schedules programmatically, with authentication and role-based access control.

### API Features

- **JWT Authentication**: Secure token-based authentication
- **Role-Based Access Control (RBAC)**: Three roles with different permissions
- **CRUD Operations**: Create, read, update, and delete schedules
- **Multi-tenant Support**: Manage schedules across multiple tenants
- **Dynamic Resource Detection**: Automatically detects CRDs in namespaces
- **Interactive Documentation**: Swagger UI and detailed HTML documentation

### Authentication

The API uses JWT (JSON Web Tokens) for authentication. Users must authenticate before accessing protected endpoints.

**Login Endpoint:**

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin123"
  }'
```

**Response:**

```json
{
  "success": true,
  "data": {
    "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refreshToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expiresIn": 3600
  }
}
```

**Using the Token:**

```bash
curl -X GET http://localhost:8080/api/v1/tenants \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

### Role-Based Access Control

The API supports three roles with different permissions:

#### 1. **admin** (Full Access)
- ✅ View all schedules
- ✅ Create schedules
- ✅ Update schedules
- ✅ Delete schedules
- ✅ Manage users (create, update, delete)
- ✅ Change user passwords and roles

#### 2. **operacion** (Operations)
- ✅ View all schedules
- ✅ Create schedules
- ✅ Update schedules
- ✅ Delete schedules
- ❌ Cannot manage users
- ❌ Cannot change passwords

#### 3. **lectura** (Read-Only)
- ✅ View all schedules
- ❌ Cannot create schedules
- ❌ Cannot update schedules
- ❌ Cannot delete schedules
- ❌ Cannot manage users

### User Management

Users are stored in Kubernetes Secrets. The default admin user can be created with:

```bash
# Create admin user secret
kubectl create secret generic kube-green-users \
  --from-literal=users="admin:\$2a\$10\$..." \
  -n keos-core

# Or use the deployment script which handles this automatically
```

**User Format in Secret:**

The secret contains a `users` key with the following format (one user per line):
```
username:password_hash:role
```

Example:
```
admin:$2a$10$...:admin
operator:$2a$10$...:operacion
viewer:$2a$10$...:lectura
```

### API Endpoints

#### Authentication
- `POST /api/v1/auth/login` - Authenticate and get JWT tokens
- `POST /api/v1/auth/refresh` - Refresh access token
- `GET /api/v1/auth/me` - Get current user information

#### Schedules
- `GET /api/v1/tenants` - List all tenants
- `GET /api/v1/schedules` - List all schedules
- `GET /api/v1/schedules/:tenant` - Get schedule for a tenant
- `POST /api/v1/schedules` - Create a new schedule
- `PUT /api/v1/schedules/:tenant` - Update a schedule
- `DELETE /api/v1/schedules/:tenant` - Delete a schedule

#### User Management (Admin only)
- `GET /api/v1/users` - List all users
- `POST /api/v1/users` - Create a new user
- `PUT /api/v1/users/:username/password` - Update user password
- `PUT /api/v1/users/:username/role` - Update user role
- `DELETE /api/v1/users/:username` - Delete a user

#### Information
- `GET /api/v1/suspended` - Get all suspended services
- `GET /api/v1/next` - Get all next operations
- `GET /api/v1/:tenant/suspended` - Get suspended services for a tenant
- `GET /api/v1/:tenant/next` - Get next operation for a tenant

### API Documentation

The API includes comprehensive documentation:

- **Swagger UI**: `http://localhost:8080/swagger` - Interactive API documentation
- **HTML Documentation**: `http://localhost:8080/docs` - Detailed documentation with CI/CD examples

The HTML documentation includes:
- Authentication flow
- All endpoints with examples
- CI/CD pipeline examples (Bash, Python, GitLab, GitHub Actions, Jenkins, Azure DevOps)
- Practical use cases (single/multiple tenants, namespaces, weekdays, weekends, etc.)

### Configuration

Enable authentication by setting environment variables:

```bash
# Backend
AUTH_ENABLED=true
JWT_SECRET=your-secret-key-here
POD_NAMESPACE=keos-core

# Frontend
VITE_AUTH_ENABLED=true
VITE_API_SERVICE_NAME=kube-green-api
VITE_API_SERVICE_PORT=8080
```

### Example: Creating a Schedule via API

```bash
# 1. Authenticate
TOKEN=$(curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' \
  | jq -r '.data.accessToken')

# 2. Create schedule
curl -X POST http://localhost:8080/api/v1/schedules \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant": "bdadevprd",
    "namespaces": ["datastores"],
    "timeSleep": "22:00",
    "timeWake": "06:00",
    "weekdaysSleep": "5",
    "weekdaysWake": "1",
    "userTimezone": "America/Bogota",
    "scheduleName": "weekend-schedule",
    "description": "Weekend shutdown schedule"
  }'
```

## Web Frontend

This fork includes a modern React-based web frontend for managing schedules through a user-friendly interface.

### Features

- **Authentication UI**: Login page with JWT support
- **Dashboard**: Overview of all tenants and schedules
- **Schedule Management**: Create, edit, and delete schedules
- **User Management**: Admin panel for managing users (admin role only)
- **Role-Based UI**: Interface adapts based on user role
- **Real-time Updates**: Shows suspended services and next operations

### Accessing the Frontend

The frontend is available at the configured service URL (default: `http://localhost:3000`).

### Frontend Features by Role

**Admin:**
- Full access to all features
- User management panel
- Can create, edit, and delete schedules
- Can manage other users

**Operacion:**
- Can view and manage schedules
- Cannot access user management
- Cannot create or modify users

**Lectura:**
- Read-only access
- Can view schedules but cannot modify them
- No access to user management

## Security Considerations

### Authentication

- JWT tokens are used for secure authentication
- Passwords are hashed using bcrypt
- Tokens have expiration times (configurable)
- Refresh tokens allow token renewal without re-authentication

### RBAC

- Role-based access control ensures users only have necessary permissions
- Admin users can manage other users and have full access
- Operations users can manage schedules but not users
- Read-only users can only view schedules

### Secrets Management

- User credentials are stored in Kubernetes Secrets
- Secrets should be properly secured using Kubernetes RBAC
- The JWT secret should be a strong, randomly generated string

## Deployment

### Prerequisites

- Kubernetes cluster (1.19+)
- kubectl configured
- Docker (for building images)
- Go 1.19+ (for development)

### Quick Deployment

1. **Apply RBAC:**

```bash
kubectl apply -f kube-green-rbac-completo.yaml
```

2. **Create authentication secret:**

```bash
# Generate password hash (use a tool or bcrypt)
# Then create secret
kubectl create secret generic kube-green-users \
  --from-literal=users="admin:\$2a\$10\$...:admin" \
  -n keos-core

# Create JWT secret
kubectl create secret generic kube-green-jwt \
  --from-literal=secret="your-random-secret-key" \
  -n keos-core
```

3. **Deploy kube-green:**

```bash
# Build and push image
docker build -t your-registry/kube-green:latest .
docker push your-registry/kube-green:latest

# Deploy using Helm or Kustomize
# (See official kube-green documentation)
```

4. **Configure environment variables:**

```yaml
env:
  - name: AUTH_ENABLED
    value: "true"
  - name: JWT_SECRET
    valueFrom:
      secretKeyRef:
        name: kube-green-jwt
        key: secret
  - name: POD_NAMESPACE
    value: "keos-core"
```

## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct, and the process for submitting pull requests to us.

## Versioning

We use [SemVer](http://semver.org/) for versioning. For the versions available, see the [release on this repository](https://github.com/kube-green/kube-green/releases).

### How to upgrade the version

To upgrade the version:

1. `make release version=v{{NEW_VERSION_TO_TAG}}` where `{{NEW_VERSION_TO_TAG}}` should be replaced with the next version to upgrade. N.B.: version should include `v` as first char.
2. `git push --tags origin v{{NEW_VERSION_TO_TAG}}`

## API Reference documentation

API reference is automatically generated with [this tool](https://github.com/ahmetb/gen-crd-api-reference-docs). To generate it automatically, are added in api versioned folder a file `doc.go` with the content of file `groupversion_info.go` and a comment with `+genclient` in the `sleepinfo_types.go` file for the resource type.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details

## Acknowledgement

Special thanks to [JGiola](https://github.com/JGiola) for the tech review.

## Give a Star! ⭐

If you like or are using this project, please give it a star. Thanks!

## Adopters

[Here](https://kube-green.dev/docs/adopters/) the list of adopters of *kube-green*.

If you already use *kube-green*, add yourself as an [adopter][add-adopters]!

[go-report-svg]: https://goreportcard.com/badge/github.com/kube-green/kube-green
[go-report-card]: https://goreportcard.com/report/github.com/kube-green/kube-green
[test-and-build-svg]: https://github.com/kube-green/kube-green/actions/workflows/test.yml/badge.svg
[test-and-build]: https://github.com/kube-green/kube-green/actions/workflows/test.yml
[coverage-badge]: https://coveralls.io/repos/github/kube-green/kube-green/badge.svg?branch=main
[coverage]: https://coveralls.io/github/kube-green/kube-green?branch=main
[website-badge]: https://img.shields.io/static/v1?label=kube-green&color=blue&message=docs&style=flat
[website]: https://kube-green.dev
[security-badge]: https://github.com/kube-green/kube-green/actions/workflows/security.yml/badge.svg
[security-pipelines]: https://github.com/kube-green/kube-green/actions/workflows/security.yml
[adopters-badge]: https://img.shields.io/static/v1?label=ADOPTERS&color=blue&message=docs&style=flat
[adopters]: https://kube-green.dev/docs/adopters/
[add-adopters]: https://github.com/kube-green/kube-green.github.io/blob/main/CONTRIBUTING.md#add-your-organization-to-adopters
[cncf-badge]: https://img.shields.io/badge/CNCF%20Landscape-5699C6
[cncf-landscape]: https://landscape.cncf.io/?item=orchestration-management--scheduling-orchestration--kube-green
