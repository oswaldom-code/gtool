package launcher.pubsub.operations;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.Map;

import org.junit.jupiter.api.Test;

import com.google.pubsub.v1.PubsubMessage;

import launcher.pubsub.PubSub.PubSubData;

class PublishMessageTest {

    private static PubSubData data(final String ceType, final String ceSource) {
        final PubSubData d = new PubSubData();
        d.message = "{\"id\":1}";
        d.ceType = ceType;
        d.ceSource = ceSource;
        d.attributes = Map.of("tenant", "es");
        d.orderingKey = "k1";
        return d;
    }

    @Test
    void buildsACloudEventWhenTypeAndSourceAreGiven() {
        final PubsubMessage msg = PublishMessage.buildMessage(data("order.created", "/orders"));

        assertEquals("order.created", msg.getAttributesMap().get("ce-type"));
        assertEquals("/orders", msg.getAttributesMap().get("ce-source"));
        assertTrue(msg.getAttributesMap().containsKey("ce-id"));
        assertEquals("es", msg.getAttributesMap().get("tenant"));
        assertEquals("application/json", msg.getAttributesMap().get("Content-Type"));
        assertEquals("k1", msg.getOrderingKey());
        assertEquals("{\"id\":1}", msg.getData().toStringUtf8());
    }

    @Test
    void optionalCloudEventFieldsDoNotFailThePublish() {
        for (final PubSubData d : new PubSubData[] {data(null, null), data("order.created", null), data(null, "/x")}) {
            final PubsubMessage msg = PublishMessage.buildMessage(d);
            assertFalse(msg.getAttributesMap().containsKey("ce-type"));
            assertEquals("es", msg.getAttributesMap().get("tenant"));
        }
    }
}
