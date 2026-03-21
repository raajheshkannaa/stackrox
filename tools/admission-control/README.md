# Admission Controller Tools

### `burst-test.sh`

Simulates a burst of deployment creates (`--dry-run=server`) against a live
cluster, scrapes admission controller Prometheus metrics before/after, and
reports metric deltas.

Run with `-h` for all options and defaults.

**Modes:**

| Mode | What it tests | Required policies |
|------|---------------|-------------------|
| `fast-path` | Spec-only policy evaluation (no image fetching) | Privileged Container, Latest tag |
| `slow-path` | Image coalescing + caching (enrichment path) | Any enrichment-required policy (e.g. Image Age) |

**Prerequisites:**

- Kubernetes/OpenShift cluster with StackRox deployed
- `admissionControl.enforcement` enabled in the SecuredCluster CR
- SecuredCluster CR deployed with `spec.monitoring.exposeEndpoint: Enabled`
- **fast-path**: Privileged Container and Latest tag policies enforced
- **slow-path**: At least one enrichment-required policy enforced (e.g. Image Age, Fixable Severity)

**Fast-path example:**

```bash
# Requires: Privileged Container + Latest tag policies enforced
VIOLATION_PCT=0   ./burst-test.sh fast-path | tee fast-baseline.txt
VIOLATION_PCT=100 ./burst-test.sh fast-path | tee fast-full.txt
VIOLATION_PCT=50  ./burst-test.sh fast-path | tee fast-mixed.txt
```

**Slow-path example (coalescing comparison):**

```bash
# 1. Deploy with MASTER image, enable an enrichment-required policy (e.g. Image Age)

# 2. Run against master
BURST_SIZE=100 UNIQUE_PCT=25 ./burst-test.sh slow-path | tee master-slowpath.txt

# 3. Swap to BRANCH image (policies persist in Central)

# 4. Run against branch
BURST_SIZE=100 UNIQUE_PCT=25 ./burst-test.sh slow-path | tee branch-slowpath.txt

# 5. Compare coalesce ratios and cache hit rates
diff master-slowpath.txt branch-slowpath.txt
```

The slow-path mode automatically runs two phases:
1. **Cold cache** -- restarts admission-control pods, then bursts. Shows coalescing effectiveness.
2. **Warm cache** -- bursts again immediately (no restart). Shows cache hit rates.

**TODO:** The scannable image pool currently contains only 20 images. `UNIQUE_COUNT` (`BURST_SIZE * UNIQUE_PCT / 100`) is capped to the pool size, so any combination requesting more than 20 unique images will be capped to 20. Expand the pool to support higher `UNIQUE_PCT` values at larger burst sizes.
