-- +goose Up
-- Approved starting dictionaries. Full catalog: docs/seed-catalog.md.
-- Fixed seed IDs are storage identifiers, not business rules.

INSERT INTO dog_status (dog_status_id, dog_status_name) VALUES
    (1, 'На учёте'),
    (2, 'Снята с учёта');
SELECT setval(pg_get_serial_sequence('dog_status', 'dog_status_id'), 2, true);

INSERT INTO breeding_status (breeding_status_id, breeding_status_name) VALUES
    (1, 'Запланирована'),
    (2, 'Проведена'),
    (3, 'Отменена');
SELECT setval(pg_get_serial_sequence('breeding_status', 'breeding_status_id'), 3, true);

INSERT INTO membership_status (membership_status_id, membership_status_name) VALUES
    (1, 'Активный'),
    (2, 'Неактивный');
SELECT setval(pg_get_serial_sequence('membership_status', 'membership_status_id'), 2, true);

INSERT INTO dog_breed (dog_breed_id, dog_breed_name) VALUES
    (1, 'Лабрадор-ретривер'),
    (2, 'Немецкая овчарка'),
    (3, 'Бигль');
SELECT setval(pg_get_serial_sequence('dog_breed', 'dog_breed_id'), 3, true);

INSERT INTO coat_color (coat_color_id, coat_color_name) VALUES
    (1, 'Чёрный'),
    (2, 'Белый'),
    (3, 'Рыжий'),
    (4, 'Чёрно-подпалый');
SELECT setval(pg_get_serial_sequence('coat_color', 'coat_color_id'), 4, true);

INSERT INTO identifier_type (identifier_type_id, identifier_type_name) VALUES
    (1, 'Микрочип'),
    (2, 'Татуировка'),
    (3, 'Идентификационная бирка');
SELECT setval(pg_get_serial_sequence('identifier_type', 'identifier_type_id'), 3, true);

INSERT INTO document_type (document_type_id, document_type_name) VALUES
    (1, 'Ветеринарный паспорт'),
    (2, 'Родословная'),
    (3, 'Регистрационное свидетельство'),
    (4, 'Ветеринарная справка'),
    (5, 'Сертификат'),
    (6, 'Диплом'),
    (7, 'Документ о результатах обследования');
SELECT setval(pg_get_serial_sequence('document_type', 'document_type_id'), 7, true);

INSERT INTO event_type (event_type_id, event_type_name) VALUES
    (1, 'Выставка'),
    (2, 'Соревнование'),
    (3, 'Занятие'),
    (4, 'Форум'),
    (5, 'Конференция'),
    (6, 'Семинар');
SELECT setval(pg_get_serial_sequence('event_type', 'event_type_id'), 6, true);

INSERT INTO event_class (event_class_id, event_class_name) VALUES
    (1, 'Класс бэби'),
    (2, 'Класс щенков'),
    (3, 'Класс юниоров'),
    (4, 'Промежуточный класс'),
    (5, 'Открытый класс'),
    (6, 'Класс чемпионов'),
    (7, 'Класс ветеранов');
SELECT setval(pg_get_serial_sequence('event_class', 'event_class_id'), 7, true);

INSERT INTO grade (grade_id, grade_name) VALUES
    (1, 'Отлично'),
    (2, 'Очень хорошо'),
    (3, 'Хорошо'),
    (4, 'Удовлетворительно');
SELECT setval(pg_get_serial_sequence('grade', 'grade_id'), 4, true);

INSERT INTO title (title_id, title_name) VALUES
    (1, 'Юный чемпион'),
    (2, 'Чемпион'),
    (3, 'Гранд Чемпион'),
    (4, 'Чемпион породы'),
    (5, 'Интернациональный чемпион');
SELECT setval(pg_get_serial_sequence('title', 'title_id'), 5, true);

INSERT INTO training_type (training_type_id, training_type_name) VALUES
    (1, 'Обучение послушанию'),
    (2, 'Подготовка к соревнованиям'),
    (3, 'Подготовка к выставкам'),
    (4, 'Социализация');
SELECT setval(pg_get_serial_sequence('training_type', 'training_type_id'), 4, true);

INSERT INTO contact_type (contact_type_id, contact_type_name) VALUES
    (1, 'Телефон'),
    (2, 'Email');
SELECT setval(pg_get_serial_sequence('contact_type', 'contact_type_id'), 2, true);

INSERT INTO address_type (address_type_id, address_type_name) VALUES
    (1, 'Адрес проживания');
SELECT setval(pg_get_serial_sequence('address_type', 'address_type_id'), 1, true);

INSERT INTO role (role_id, role_name) VALUES
    (1, 'Администратор'),
    (2, 'Сотрудник клуба'),
    (3, 'Ветеринар'),
    (4, 'Эксперт-кинолог'),
    (5, 'Владелец собаки'),
    (6, 'Руководитель клуба');
SELECT setval(pg_get_serial_sequence('role', 'role_id'), 6, true);

INSERT INTO sex (sex_id, sex_name) VALUES
    (1, 'Кобель'),
    (2, 'Сука');
SELECT setval(pg_get_serial_sequence('sex', 'sex_id'), 2, true);

INSERT INTO disease (disease_id, disease_name) VALUES
    (1, 'Чума плотоядных'),
    (2, 'Парвовирусная инфекция'),
    (3, 'Аденовирусная инфекция'),
    (4, 'Бешенство'),
    (5, 'Лептоспироз'),
    (6, 'Бордетеллиоз'),
    (7, 'Парагрипп');
SELECT setval(pg_get_serial_sequence('disease', 'disease_id'), 7, true);

-- +goose Down
-- Delete only owned seed IDs; references intentionally prevent unsafe rollback.
DELETE FROM disease WHERE disease_id IN (1, 2, 3, 4, 5, 6, 7);
DELETE FROM sex WHERE sex_id IN (1, 2);
DELETE FROM role WHERE role_id IN (1, 2, 3, 4, 5, 6);
DELETE FROM address_type WHERE address_type_id IN (1);
DELETE FROM contact_type WHERE contact_type_id IN (1, 2);
DELETE FROM training_type WHERE training_type_id IN (1, 2, 3, 4);
DELETE FROM title WHERE title_id IN (1, 2, 3, 4, 5);
DELETE FROM grade WHERE grade_id IN (1, 2, 3, 4);
DELETE FROM event_class WHERE event_class_id IN (1, 2, 3, 4, 5, 6, 7);
DELETE FROM event_type WHERE event_type_id IN (1, 2, 3, 4, 5, 6);
DELETE FROM document_type WHERE document_type_id IN (1, 2, 3, 4, 5, 6, 7);
DELETE FROM identifier_type WHERE identifier_type_id IN (1, 2, 3);
DELETE FROM coat_color WHERE coat_color_id IN (1, 2, 3, 4);
DELETE FROM dog_breed WHERE dog_breed_id IN (1, 2, 3);
DELETE FROM membership_status WHERE membership_status_id IN (1, 2);
DELETE FROM breeding_status WHERE breeding_status_id IN (1, 2, 3);
DELETE FROM dog_status WHERE dog_status_id IN (1, 2);
