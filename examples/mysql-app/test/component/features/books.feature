@smoke
Feature: Books stored in MySQL from data/books.json

  Background:
    * def MySql = Java.type('launcher.mysql.MySqlClient')
    * def mysql = new MySql({ url: 'jdbc:mysql://localhost:3306/test', user: 'root', password: 'root' })

  Scenario: the app inserts every book from the data file
    * def clrs = mysql.getRow('books', { isbn: '978-0262033848' })
    * match clrs == { isbn: '978-0262033848', title: 'Introduction to Algorithms', copies: 4 }
    * def gof = mysql.getRow('books', { isbn: '978-0201633610' })
    * match gof == { isbn: '978-0201633610', title: 'Design Patterns', copies: 2 }
