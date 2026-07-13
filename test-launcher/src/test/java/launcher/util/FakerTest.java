package launcher.util;

import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.UUID;

import org.junit.jupiter.api.Test;

class FakerTest {

    private final Faker faker = new Faker();

    @Test
    void uuidIsParseable() {
        UUID.fromString(faker.uuid()); // throws if invalid
    }

    @Test
    void emailLooksLikeEmail() {
        assertTrue(faker.email().contains("@"));
    }

    @Test
    void numberBetweenIsInRange() {
        final long n = faker.numberBetween(5, 10);
        assertTrue(n >= 5 && n < 10);
    }

    @Test
    void fullNameNotNull() {
        assertNotNull(faker.fullName());
    }
}
