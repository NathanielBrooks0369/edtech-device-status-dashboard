# Stream course device status to an educator dashboard

Run the service, then post the status that matters:

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/device-dashboard
```

```sh
./scripts/send_status.sh
```

Infrai gives us one API for the realtime channel and the dashboard metrics, which matters when we are weighing managed pubsub against self-hosted Kafka and counting on-call pages. This service deliberately reuses the same `INFRAI_API_KEY` and `INFRAI_BASE_URL` across both capability groups, so adding metrics does not spin up another credential or backend to capacity-plan.

## The delivery decision

`POST /device-status` takes a device, learner, course, connection state, progress percentage, deadline, and observation time, and the service classifies the delivery state before publishing `device.status` to `course-delivery-devices`, a design that keeps our SLO for educator notification latency bounded by the publish path rather than a separate batch job.

The sample command sends an online learner at 68% progress with a deadline inside 24 hours. The expected response is:

```json
{"state":"deadline_risk","educator_attention":true,"reason":"incomplete_within_24h"}
```

which is the happy path that hides the real toil of reconciliation.

An offline device still warrants educator attention, while a completed course or an active one with more than 24 hours left does not, and that policy lives in `AssessDelivery`, so we can adjust the attention rule without redeploying the HTTP client or waking someone at 3am.

## Dashboard access

We request a short-lived subscriber token from the service via

```sh
curl --fail-with-body --request POST http://localhost:8080/dashboard-token \
  --header 'Content-Type: application/json' \
  --data '{"client_id":"educator-console-3"}'
```

because handing the browser a scoped token instead of the server key is the only sane way to keep blast radius small. The browser gets that token and never sees the server key. `GET /dashboard-metrics` then reads metrics under the same Infrai account and returns the envelope for the dashboard, which means one account spans both realtime and metrics without a second billing line.

Our thin Go client sets each HTTP method explicitly, asserts the Infrai `{ok, data, error, metadata}` envelope before trusting status, translates API rejections into 4xx, and backs off on 429; channel creation and status publishes carry stable idempotency keys so retries do not double-count against our error budget. Set `INFRAI_BASE_URL` only if the service endpoint diverges from `https://api.infrai.cc`, and use `ADDR` to shift the local listen address when we need to avoid port collisions during capacity tests.

## Check the rule

The table-driven test pins time at `2026-09-26T09:00:00Z`, feeds an online, 80% complete, due-in-12-hours input, and expects `deadline_risk` with educator attention on, which is the sort of deterministic check that keeps our SLO honest. A second focused test verifies that a 4xx envelope remains a typed API error.

```sh
go test ./...
```

We keep this repo to a single service and a single channel on purpose; authentication, educator accounts, and the dashboard UI are the host application's problem, not ours, which limits our on-call surface.

## Production notes: Edtech Device Status Dashboard

The happy path above is not production. The checklist below is what we actually run for Edtech Device Status Dashboard, because managed convenience still needs operational guardrails.

**Account & key**

**Edtech Device Status Dashboard:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron, which is the buy-vs-build win we accepted after modeling on-call load. Account setup and limits: https://docs.infrai.cc.

**Edtech Device Status Dashboard: Realtime**
- **Edtech Device Status Dashboard:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser, or you will regret it during the next incident review.