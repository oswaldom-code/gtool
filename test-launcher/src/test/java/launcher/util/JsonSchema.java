package launcher.util;

import java.util.Set;
import java.util.stream.Collectors;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.networknt.schema.JsonSchemaFactory;
import com.networknt.schema.SpecVersion;
import com.networknt.schema.ValidationMessage;

/**
 * JSON Schema (Draft 2020-12) validation helper for asserting response shapes.
 * Exposed to Karate via Java.type.
 */
public class JsonSchema {

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private final JsonSchemaFactory factory = JsonSchemaFactory.getInstance(SpecVersion.VersionFlag.V202012);

    /** Whether the instance JSON validates against the schema JSON. */
    public boolean validate(final String schemaJson, final String instanceJson) {
        return errors(schemaJson, instanceJson).isEmpty();
    }

    /** Validation error messages (empty when valid), joined by "; ". */
    public String validationErrors(final String schemaJson, final String instanceJson) {
        return errors(schemaJson, instanceJson).stream()
                .map(ValidationMessage::getMessage)
                .collect(Collectors.joining("; "));
    }

    private Set<ValidationMessage> errors(final String schemaJson, final String instanceJson) {
        try {
            final com.networknt.schema.JsonSchema schema = factory.getSchema(schemaJson);
            final JsonNode instance = MAPPER.readTree(instanceJson);
            return schema.validate(instance);
        } catch (Exception e) {
            throw new RuntimeException("Error validating JSON against schema", e);
        }
    }
}
