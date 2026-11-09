package launcher.pubsub;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.Map;

import org.junit.jupiter.api.Test;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.google.pubsub.v1.PubsubMessage;

import launcher.pubsub.PubSub.PubSubData;

class UtilsTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private static JsonNode json(final String text) throws Exception {
        return MAPPER.readTree(text);
    }

    private static PubSubData expecting(final Map<String, String> attributes) {
        final PubSubData data = new PubSubData();
        data.attributes = attributes;
        return data;
    }

    private static PubsubMessage withAttributes(final Map<String, String> attributes) {
        return PubsubMessage.newBuilder().putAllAttributes(attributes).build();
    }

    @Test
    void containsAttributesAcceptsAMessageWithEveryExpectedAttribute() {
        final PubsubMessage msg = withAttributes(Map.of("type", "order", "region", "es", "extra", "x"));
        assertTrue(Utils.containsAttributes(expecting(Map.of("type", "order", "region", "es")), msg));
    }

    @Test
    void containsAttributesRejectsAWrongOrMissingAttribute() {
        final PubsubMessage msg = withAttributes(Map.of("type", "order"));
        assertFalse(Utils.containsAttributes(expecting(Map.of("type", "refund")), msg));
        assertFalse(Utils.containsAttributes(expecting(Map.of("type", "order", "region", "es")), msg));
    }

    @Test
    void containsAttributesWithoutExpectationsMatchesAnything() {
        final PubsubMessage msg = withAttributes(Map.of("type", "order"));
        assertTrue(Utils.containsAttributes(expecting(null), msg));
        assertTrue(Utils.containsAttributes(expecting(Map.of()), msg));
    }

    @Test
    void compareJsonNodesMissingFieldIsFalseNotAnException() throws Exception {
        assertFalse(Utils.compareJsonNodes(json("{\"a\": [1]}"), json("{\"b\": [1]}")));
        assertFalse(Utils.compareJsonNodes(json("{\"a\": {\"x\": 1}}"), json("{\"b\": {\"x\": 1}}")));
    }

    @Test
    void compareJsonNodesNullIsAWildcard() throws Exception {
        assertTrue(Utils.compareJsonNodes(json("{\"id\": null, \"n\": 1}"), json("{\"id\": \"abc\", \"n\": 1}")));
        assertTrue(Utils.compareJsonNodes(json("[{\"id\": null}]"), json("[{\"id\": 7}]")), "wildcard inside arrays");
    }

    @Test
    void compareJsonNodesArraysIgnoreOrderButCountDuplicates() throws Exception {
        assertTrue(Utils.compareJsonNodes(json("[1, 2, 2]"), json("[2, 1, 2]")));
        assertFalse(Utils.compareJsonNodes(json("[1, 1, 2]"), json("[2, 1]")));
        assertFalse(Utils.compareJsonNodes(json("[1, 1, 2]"), json("[1, 2, 2]")));
    }
}
