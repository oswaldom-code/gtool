function fn() {

  var env = karate.env; // get system property 'karate.env'
  karate.log('karate.env system property was:', env);

  var config = {};

  // Sleep helper: sleep(seconds)
  var sleep = function (seconds) {
    java.lang.Thread.sleep(seconds * 1000);
  };
  config.sleep = sleep;

  // PostgreSQL helper (assertions / seeding against a running Postgres on localhost)
  var PostgresClient = Java.type('launcher.postgres.PostgresClient');
  config.ps = new PostgresClient({
    url: 'jdbc:postgresql://localhost:5432/postgres',
    user: 'postgres',
    password: 'postgres'
  });

  // Date helper
  var DateUtils = Java.type('utils.DateUtils');
  config.du = new DateUtils();

  // PDF comparison helper
  var PdfUtils = Java.type('utils.PdfUtils');
  config.pdfu = new PdfUtils();

  return config;
}
