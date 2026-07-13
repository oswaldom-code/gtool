package launcher.util;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.Map;

import org.junit.jupiter.api.Test;

class JwtTest {

    private final Jwt jwt = new Jwt();

    @Test
    void generateAndVerifyRoundtrip() {
        final String token = jwt.generateHs256("secret", Map.of("sub", "user-1", "role", "admin"));
        assertTrue(jwt.verifyHs256(token, "secret"));
    }

    @Test
    void verifyFailsWithWrongSecret() {
        final String token = jwt.generateHs256("secret", Map.of("sub", "user-1"));
        assertFalse(jwt.verifyHs256(token, "other-secret"));
    }

    @Test
    void getClaimReturnsValue() {
        final String token = jwt.generateHs256("secret", Map.of("role", "admin"));
        assertEquals("admin", jwt.getClaim(token, "role"));
    }

    @Test
    void decodePayloadContainsClaim() {
        final String token = jwt.generateHs256("secret", Map.of("sub", "user-1"));
        assertTrue(jwt.decodePayload(token).contains("user-1"));
    }
}
