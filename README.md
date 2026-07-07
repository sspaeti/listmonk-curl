# sub-subscribe

Curl-friendly newsletter signup shim in front of [listmonk](https://listmonk.app/).

```sh
# Short form — no flags, email in path
curl https://sub.ssp.sh/you@example.com

# Standard POST
curl -d "email=you@example.com" https://sub.ssp.sh

# Subscriber count
curl sub.ssp.sh/count

# Self-hosting manifesto
curl sub.ssp.sh/why
```

Browser `GET /` redirects to the listmonk subscription form.

## Deploy on Railway

1. Create a new Railway service pointing at this directory.
2. Set the environment variables (see `.env.example`):

   | Variable | Required | Description |
   |---|---|---|
   | `SUB_LIST_UUID` | yes | List UUID — listmonk admin → Lists |
   | `SUB_API_USER` | for `/count` | listmonk API username |
   | `SUB_API_TOKEN` | for `/count` | listmonk API token |
   | `SUB_LISTMONK_URL` | no | listmonk base URL (default `https://list.ssp.sh`) |
   | `SUB_FORM_URL` | no | Browser redirect target (default `https://list.ssp.sh/subscription/form`) |
   | `PORT` | no | Set automatically by Railway |

3. Add `sub.ssp.sh` as a custom domain in the Railway service settings.
4. Add a DNS CNAME: `sub.ssp.sh` → your Railway service domain.

## Why a separate service

The shim is intentionally isolated from the main listmonk deployment. If the shim crashes or is redeployed, listmonk is unaffected. listmonk updates don't touch the shim.

## Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/<email>` | Subscribe (short form, email in path) |
| `POST` | `/` | Subscribe (`email` + optional `name` form fields) |
| `GET` | `/` | Usage text (curl) or redirect to form (browser) |
| `GET` | `/count` | Confirmed subscriber count, CORS-open |
| `GET` | `/why` | ASCII self-hosting manifesto |
