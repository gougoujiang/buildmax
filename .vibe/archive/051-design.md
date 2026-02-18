# Design 051: Add user registration

## Goal

Enable new users to register via the portal (sign up with email) and unify login/signup behind an MVP OTP flow: backend accepts hardcoded OTP `123456`, no real email sending. Same endpoint requests OTP for both flows; intent distinguishes "user not found" (login) from "email already registered" (signup).

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/store** | User persistence | User model (unchanged), UserStore extended with CreateUser, Store implementation |
| **internal/server** | HTTP API | OTP request handler, login handler (email + otp), OpenAPI spec |
| **portal** | Auth UX | SignUp page, Login page (two-step: email → OTP), api.requestOtp, api.login(email, otp), routing |

## Structure

**Backend**

- `internal/store/store.go`
  - Extend `UserStore` with `CreateUser(ctx, email string) (*User, error)`.
  - Implement `CreateUser` on `Store`: generate `user_id` via `id.New()`, set `Email`, `Name` to empty string (or optional: local part of email), `CreatedAt` from `time.Now().Unix()`. On duplicate email (GORM unique constraint), return a sentinel error (e.g. `ErrEmailExists`) so server can map to 409.
- `internal/store/`: add tests for CreateUser (success, duplicate email returns error).

- `internal/server/`
  - New file or add to `login.go`: `otpRequestHandler`, `OtpRequestRequest`, `OtpRequestResponse` types. Register `POST /api/otp/request` in `server.go`.
  - Update `loginHandler`: accept body `{ "email", "otp" }`; validate OTP against constant `123456`; if wrong or missing OTP return 401 "invalid otp"; rest unchanged (UserByEmail → JWT → response).
  - Define constant for hardcoded OTP (e.g. `const otpCode = "123456"`) in one place.
  - Update `static/openapi.json`: add `/api/otp/request`, update `/api/login` request body to include `otp`.

**Portal**

- `portal/src/`
  - New page: `pages/SignUp.tsx` — form email → "Get OTP" → call `requestOtp(email, "signup")`; on 409 show "email already registered" + link to login; on 200 show "OTP sent" and OTP input step; on submit call `login(email, otp)` then setAuth and navigate into app.
  - Update `pages/Login.tsx`: step 1 — email + "Get OTP" → `requestOtp(email, "login")`; on 404 (or 400 "user not found") show "user not found / please sign up"; on 200 show "OTP sent" and OTP input; step 2 — submit `login(email, otp)` then setAuth.
  - `lib/api.ts`: add `requestOtp(email: string, intent: "signup" | "login"): Promise<OtpRequestResponse>`; change `login(email)` to `login(email: string, otp: string)`.
  - Routing: when unauthenticated, show either Login or SignUp (e.g. hash `#/login`, `#/signup`); link "Sign up" on Login and "Sign in" on SignUp. App.tsx or router: if no token and route is signup render SignUp, else Login.

## Method design

| Package / layer | Component | Method / handler | Signature / contract |
|-----------------|-----------|------------------|----------------------|
| **store** | UserStore | CreateUser | `CreateUser(ctx context.Context, email string) (*User, error)`. Creates user with new user_id (id.New()), given email, name "". Returns ErrEmailExists (or equivalent) when email already exists so server can respond 409. |
| **store** | Store | CreateUser | Implementation: insert User; on duplicate key error return sentinel; otherwise return created user. |
| **server** | (handler) | otpRequestHandler | POST /api/otp/request. Body: `{ "email": string, "intent": "signup" \| "login" }` (intent optional, default "signup"). Email required. If intent=login and UserByEmail nil → 404 JSON "user not found". If intent=signup and user exists → 409 JSON "email already registered". Else: if user nil call CreateUser (signup); then return 200 `{ "message": "otp_sent" }`. |
| **server** | (handler) | loginHandler | POST /api/login. Body: `{ "email": string, "otp": string }`. Email and otp required. If user not found → 401 "user not found". If otp != "123456" → 401 "invalid otp". Else issue JWT, return 200 `{ "token", "user" }`. |
| **portal** | api | requestOtp | `requestOtp(email: string, intent: "signup" \| "login"): Promise<{ message: string }>`. POST body `{ email, intent }`. Throws on non-2xx; message from JSON or status. |
| **portal** | api | login | `login(email: string, otp: string): Promise<LoginResponse>`. POST body `{ email, otp }`. Same as today for success path. |
| **portal** | SignUp page | — | Email input → Get OTP → requestOtp(email, "signup"); on 409 show error + link to login; on 200 show OTP input → login(email, otp) → setAuth. |
| **portal** | Login page | — | Email input → Get OTP → requestOtp(email, "login"); on 404/400 show "user not found" / "please sign up"; on 200 show OTP input → login(email, otp) → setAuth. |

## How they work together

**Signup flow**

1. User opens Sign Up, enters email, clicks "Get OTP".
2. Portal calls `requestOtp(email, "signup")` → POST /api/otp/request with `{ email, intent: "signup" }`.
3. Server: if user exists → 409 "email already registered". If not → CreateUser → 200 `{ "message": "otp_sent" }`.
4. Portal shows "OTP sent" and OTP field; user enters 123456, submits; portal calls `login(email, otp)` → POST /api/login; server validates OTP, returns JWT; portal setAuth and shows app.

**Login flow**

1. User opens Login, enters email, clicks "Get OTP".
2. Portal calls `requestOtp(email, "login")` → POST /api/otp/request with `{ email, intent: "login" }`.
3. Server: if user not found → 404 "user not found". If found → 200 `{ "message": "otp_sent" }`.
4. Portal shows OTP step; user enters 123456, submits; same as signup step 4.

**Duplicate signup**

- Same as signup step 2–3: server returns 409; portal shows "email already registered" and link to login.

**Store**

- CreateUser is used only by otpRequestHandler when intent is signup (or default) and user does not exist. UserByEmail is used by both otpRequestHandler (to decide 409 vs create) and loginHandler (to get user for JWT).

## Errors and status codes

- **store**: Define a sentinel error (e.g. `var ErrEmailExists = errors.New("email already exists")`) for CreateUser so server can detect duplicate and respond 409. Other DB errors propagate as 500.
- **server**: 400 empty email/otp or invalid body; 401 login user not found or invalid otp; 404 otp/request with intent=login and user not found; 409 otp/request with intent=signup and email exists; 503 login/otp not configured (UserStore or JWTSecret nil).

## OpenAPI

- **POST /api/otp/request**: requestBody required `email`, optional `intent` ("signup" | "login"). Responses: 200 `{ "message": "otp_sent" }`, 404 "user not found" (login intent), 409 "email already registered" (signup intent), 400 invalid/empty email.
- **POST /api/login**: requestBody required `email`, `otp`. Responses: 200 token + user, 401 "user not found" or "invalid otp", 400 invalid body.

## Tests

- **store**: Test CreateUser success (user created, UserByEmail finds it); Test CreateUser duplicate email (second CreateUser same email returns ErrEmailExists or equivalent).
- **server**: Test otpRequestHandler: signup new user → 200; signup existing email → 409; login unknown email → 404; login existing user → 200. Test loginHandler: valid email+otp 123456 → 200; wrong otp → 401; missing otp → 400; user not found → 401. Extend mockUserStore with CreateUser for tests (in-memory map + create logic).

---

## Changes for review

| Area | Change |
|------|--------|
| **internal/store** | UserStore: add `CreateUser(ctx, email string) (*User, error)`. Store: implement CreateUser; add sentinel ErrEmailExists. store_test: CreateUser success and duplicate. |
| **internal/server** | Add OtpRequestRequest/Response, otpRequestHandler; register POST /api/otp/request. login.go: LoginRequest add Otp field; loginHandler require and validate otp constant "123456". helpers_test: mockUserStore add CreateUser. login_test + new otp_test (or in login_test): cases for otp and otp/request. static/openapi.json: add /api/otp/request, update /api/login. |
| **portal** | api.ts: requestOtp(email, intent), login(email, otp). New SignUp.tsx. Update Login.tsx to two-step (Get OTP → OTP input). App or routing: #/signup vs #/login, render SignUp or Login; links between them. |
