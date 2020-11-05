#!/usr/bin/env bash
export DOCKERIMAGE=${DOCKERIMAGE:-syncano/acme-proxy}
export VERSION="$2"

TARGET="$1"
PUSH=true
MIGRATIONS=false

usage() { echo "* Usage: $0 <environment> <version> [--skip-gitlog][--skip-push][--migration]" >&2; exit 1; }
[[ -n $TARGET ]] || usage
[[ -n $VERSION ]] || usage

set -euo pipefail

if ! command -v kubectl > /dev/null; then
    echo "! kubectl not installed" >&2; exit 1
fi

if [[ ! -f "deploy/env/${TARGET}.env" ]]; then
    echo "! environment ${TARGET} does not exist in deploy/env/"; exit 1
fi


# Parse last git message (for PR integration).
GITLOG=$(git log -1)
[[ $GITLOG == *"[migration]"* ]] && MIGRATIONS=true

# Parse arguments.
for PARAM in "${@:3}"; do
    case $PARAM in
        --migration)
          MIGRATIONS=true
          ;;
        --skip-push)
          PUSH=false
          ;;
        *)
          usage
          ;;
    esac
done

envsubst() {
    for var in $(compgen -e); do
        echo "$var: \"${!var//\"/\\\"}\""
    done | PYTHONWARNINGS=ignore jinja2 "$1"
}


echo "* Starting deployment for $TARGET at $VERSION for $DOCKERIMAGE."

# Setup environment variables.
set -a
# shellcheck disable=SC1090
source deploy/env/"${TARGET}".env
set +a
BUILDTIME=$(date +%Y-%m-%dT%H%M)
export BUILDTIME


# Push docker image.
if $PUSH; then
    echo "* Tagging $DOCKERIMAGE $VERSION."
    docker tag "$DOCKERIMAGE" "$DOCKERIMAGE":"$VERSION"

    echo "* Pushing $DOCKERIMAGE:$VERSION."
    docker push "$DOCKERIMAGE":"$VERSION"
fi

IMAGE="$DOCKERIMAGE":"$VERSION"
export IMAGE


# Create configmap.
echo "* Updating ConfigMap."
CONFIGMAP="apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: acme-proxy\ndata:\n"
while read -r line; do
    if [[ -n "${line}" && "${line}" != "#"* ]]; then
        CONFIGMAP+="  ${line%%=*}: \"${line#*=}\"\n"
    fi
done < deploy/env/"${TARGET}".env
echo -e "$CONFIGMAP" | kubectl apply -f -

kubectl create configmap acme-proxy-routes --from-file=deploy/routes.yaml -o yaml --dry-run | kubectl apply -f -


# Migrate database.
if $MIGRATIONS; then
    echo "* Starting migration job."
    kubectl delete job/acme-proxy-migration 2>/dev/null || true
    envsubst deploy/yaml/migration-job.yml.j2 | kubectl apply -f -

    for _ in {1..300}; do
        echo ". Waiting for migration job."
        sleep 1
        PODNAME=$(kubectl get pods -l job-name=acme-proxy-migration -a --sort-by=.status.startTime -o name 2>/dev/null | tail -n1)
        [ -z "$PODNAME" ] && continue

        kubectl attach "$PODNAME" 2> /dev/null || true
        SUCCESS=$(kubectl get jobs acme-proxy-migration -o jsonpath='{.status.succeeded}' 2>/dev/null | grep -v 0 || true)
        [ -n "$SUCCESS" ] && break
    done

    if [ -z "$SUCCESS" ]; then
        echo "! Migration failed!"
        exit 1
    fi
    kubectl delete job/acme-proxy-migration
fi

# Start with deployment (server).
echo "* Deploying Server."
REPLICAS=$(kubectl get deployment/acme-proxy -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "${WEB_MIN}")
export REPLICAS
envsubst deploy/yaml/server-deployment.yml.j2 | kubectl apply -f -
envsubst deploy/yaml/server-hpa.yml.j2 | kubectl apply -f -

echo "* Deploying Service for Server."
envsubst deploy/yaml/server-internal-service.yml.j2 | kubectl apply -f -
envsubst deploy/yaml/server-service.yml.j2 | kubectl apply -f -

# Wait for server deployments to finish.
echo
echo ". Waiting for deployment to finish..."
kubectl rollout status deployment/acme-proxy
