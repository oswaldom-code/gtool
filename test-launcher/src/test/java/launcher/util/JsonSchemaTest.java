package launcher.util;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import org.junit.jupiter.api.Test;

class JsonSchemaTest {

    private final JsonSchema jsonSchema = new JsonSchema();

    private static final String SCHEMA = "{"
            + "\"type\":\"object\","
            + "\"required\":[\"name\"],"
            + "\"properties\":{\"name\":{\"type\":\"string\"},\"age\":{\"type\":\"integer\"}}"
            + "}";

    @Test
    void validInstancePasses() {
        assertTrue(jsonSchema.validate(SCHEMA, "{\"name\":\"ada\",\"age\":36}"));
    }

    @Test
    void missingRequiredFails() {
        assertFalse(jsonSchema.validate(SCHEMA, "{\"age\":36}"));
    }

    @Test
    void wrongTypeFails() {
        assertFalse(jsonSchema.validate(SCHEMA, "{\"name\":\"ada\",\"age\":\"old\"}"));
    }

    @Test
    void errorsReportedForInvalid() {
        assertFalse(jsonSchema.validationErrors(SCHEMA, "{\"age\":36}").isEmpty());
    }
}
