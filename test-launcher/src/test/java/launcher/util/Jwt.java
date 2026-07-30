package launcher.util;

import java.util.Base64;
import java.util.Map;

import com.auth0.jwt.JWT;
import com.auth0.jwt.algorithms.Algorithm;
import com.auth0.jwt.exceptions.JWTVerificationException;

/**
 * JWT helper for generating, verifying and inspecting HS256 tokens in tests.
 * Exposed to Karate via Java.type.
 */
public class Jwt {

    /** Create an HS256-signed token from a claims map. */
    public String generateHs256(final String secret, final Map<String, Object> claims) {
        return JWT.create().withPayload(claims).sign(Algorithm.HMAC256(secret));
    }

    /** Verify an HS256 token signature against a secret. */
    public boolean verifyHs256(final String token, final String secret) {
        try {
            JWT.require(Algorithm.HMAC256(secret)).build().verify(token);
            return true;
        } catch (JWTVerificationException e) {
            return false;
        }
    }

    /** Read a string claim from a token (no signature verification). */
    public String getClaim(final String token, final String name) {
        return JWT.decode(token).getClaim(name).asString();
    }

    /** Return the decoded JSON payload of a token (no signature verification). */
    public String decodePayload(final String token) {
        final String payload = JWT.decode(token).getPayload();
        return new String(Base64.getUrlDecoder().decode(payload));
    }
}
