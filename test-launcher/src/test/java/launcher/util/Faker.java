package launcher.util;

/**
 * Random test-data generator (wraps datafaker). Exposed to Karate via Java.type.
 */
public class Faker {

    private final net.datafaker.Faker faker = new net.datafaker.Faker();

    public String fullName() {
        return faker.name().fullName();
    }

    public String firstName() {
        return faker.name().firstName();
    }

    public String lastName() {
        return faker.name().lastName();
    }

    public String email() {
        return faker.internet().emailAddress();
    }

    public String uuid() {
        return faker.internet().uuid();
    }

    public String word() {
        return faker.lorem().word();
    }

    public long numberBetween(final long min, final long max) {
        return faker.number().numberBetween(min, max);
    }

    /** Evaluate a datafaker expression, e.g. {@code "#{name.fullName}"}. */
    public String expression(final String expr) {
        return faker.expression(expr);
    }
}
