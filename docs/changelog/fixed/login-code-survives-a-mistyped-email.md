- Signing in with a single-use login code no longer spends the code when the
  email address does not match it. The code was redeemed before the address was
  checked, so one mistyped or autofilled address burned it: the retry with the
  right address failed too, and an operator had to issue another code. The
  address is resolved first and the code is only redeemed for that account, and
  the server now logs why a code was rejected — the response still says nothing
  more than `invalid otp`. Whitespace around a pasted address or code is also
  ignored now, rather than failing the same opaque way.
