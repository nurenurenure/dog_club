-- +goose Up
-- Audited PostgreSQL 18 baseline. See docs/database-audit.md.
-- Original types, nullability and existing deletion policies are retained.

CREATE TABLE dog (
    dog_id serial NOT NULL,
    dog_nickname varchar(100) NOT NULL,
    dog_date_of_birth date NOT NULL,
    dog_identifier_number varchar(100),
    dog_identifier_date date,
    dog_status_id integer NOT NULL,
    identifier_type_id integer NOT NULL,
    dog_breed_id integer NOT NULL,
    coat_color_id integer NOT NULL,
    sex_id integer NOT NULL,
    CONSTRAINT pk_dog PRIMARY KEY (dog_id),
    CONSTRAINT uq_dog_identifier_number UNIQUE (dog_identifier_number)
) WITH (autovacuum_enabled = true);

CREATE TABLE club_member (
    club_member_id serial NOT NULL,
    club_member_full_name varchar(150) NOT NULL,
    club_member_date_of_birth date,
    club_member_membership_date timestamp(6),
    role_id integer NOT NULL,
    membership_status_id integer NOT NULL,
    CONSTRAINT pk_club_member PRIMARY KEY (club_member_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE document (
    document_id serial NOT NULL,
    document_number varchar(50),
    document_given_by text,
    document_issue_date date,
    document_expiration_date date,
    dog_id integer NOT NULL,
    club_member_id integer NOT NULL,
    document_type_id integer NOT NULL,
    CONSTRAINT pk_document PRIMARY KEY (document_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE veterinary_examination (
    veterinary_examination_id serial NOT NULL,
    veterinary_examination_date timestamp(6) NOT NULL,
    veterinary_examination_description text,
    veterinary_examination_result text,
    veterinary_examination_recommendations text,
    dog_id integer NOT NULL,
    club_member_id integer NOT NULL,
    CONSTRAINT pk_veterinary_examination PRIMARY KEY (veterinary_examination_id, dog_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE vaccination (
    vaccination_id serial NOT NULL,
    vaccination_date timestamp(6) NOT NULL,
    vaccination_next_date date,
    dog_id integer NOT NULL,
    vaccine_type_id integer NOT NULL,
    CONSTRAINT pk_vaccination PRIMARY KEY (vaccination_id, dog_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE vaccine_type (
    vaccine_type_id serial NOT NULL,
    vaccine_type_name varchar(100) NOT NULL,
    vaccine_type_maker varchar(100),
    validity_period integer,
    CONSTRAINT pk_vaccine_type PRIMARY KEY (vaccine_type_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE pedigree (
    pedigree_id serial NOT NULL,
    dog_id integer NOT NULL,
    pedigree_mother integer NOT NULL,
    pedigree_father integer NOT NULL,
    CONSTRAINT pk_pedigree PRIMARY KEY (pedigree_id, dog_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE document_type (
    document_type_id serial NOT NULL,
    document_type_name varchar(70) NOT NULL,
    CONSTRAINT pk_document_type PRIMARY KEY (document_type_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE disease (
    disease_id serial NOT NULL,
    disease_name varchar(100) NOT NULL,
    disease_description text,
    CONSTRAINT pk_disease PRIMARY KEY (disease_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE breeding (
    breeding_id serial NOT NULL,
    breeding_date timestamp(6),
    breeding_number_of_puppies integer,
    female_dog_id integer NOT NULL,
    male_dog_id integer NOT NULL,
    breeding_status_id integer NOT NULL,
    CONSTRAINT pk_breeding PRIMARY KEY (breeding_id, female_dog_id, male_dog_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE breeding_status (
    breeding_status_id serial NOT NULL,
    breeding_status_name varchar(50) NOT NULL,
    CONSTRAINT pk_breeding_status PRIMARY KEY (breeding_status_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE coat_color (
    coat_color_id serial NOT NULL,
    coat_color_name varchar(50) NOT NULL,
    CONSTRAINT pk_coat_color PRIMARY KEY (coat_color_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE title (
    title_id serial NOT NULL,
    title_name varchar(100) NOT NULL,
    CONSTRAINT pk_title PRIMARY KEY (title_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE sex (
    sex_id serial NOT NULL,
    sex_name varchar(20) NOT NULL,
    CONSTRAINT pk_sex PRIMARY KEY (sex_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE dog_breed (
    dog_breed_id serial NOT NULL,
    dog_breed_name varchar(100) NOT NULL,
    CONSTRAINT pk_dog_breed PRIMARY KEY (dog_breed_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE dog_training_session (
    dog_training_session_id serial NOT NULL,
    dog_training_session_start_date timestamp(6),
    dog_training_session_end_date timestamp(6),
    dog_id integer NOT NULL,
    club_member_id integer NOT NULL,
    training_type_id integer NOT NULL,
    CONSTRAINT pk_dog_training_session PRIMARY KEY (dog_training_session_id, dog_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE training_type (
    training_type_id serial NOT NULL,
    training_type_name varchar(100) NOT NULL,
    CONSTRAINT pk_training_type PRIMARY KEY (training_type_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE dog_status (
    dog_status_id serial NOT NULL,
    dog_status_name varchar(50) NOT NULL,
    CONSTRAINT pk_dog_status PRIMARY KEY (dog_status_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE identifier_type (
    identifier_type_id serial NOT NULL,
    identifier_type_name varchar(50),
    CONSTRAINT pk_identifier_type PRIMARY KEY (identifier_type_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE role (
    role_id serial NOT NULL,
    role_name varchar(50) NOT NULL,
    CONSTRAINT pk_role PRIMARY KEY (role_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE membership_status (
    membership_status_id serial NOT NULL,
    membership_status_name varchar(50) NOT NULL,
    CONSTRAINT pk_membership_status PRIMARY KEY (membership_status_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE address (
    address_id serial NOT NULL,
    address_locality varchar(100),
    address_street varchar(100),
    address_building_number varchar(20),
    address_apartment_number varchar(20),
    club_member_id integer NOT NULL,
    address_type_id integer NOT NULL,
    CONSTRAINT pk_address PRIMARY KEY (address_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE address_type (
    address_type_id serial NOT NULL,
    address_type_name varchar(50) NOT NULL,
    CONSTRAINT pk_address_type PRIMARY KEY (address_type_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE event (
    event_id serial NOT NULL,
    event_name varchar(100),
    event_start_date timestamp(6),
    event_end_date timestamp(6),
    event_description text,
    address_id integer NOT NULL,
    event_type_id integer NOT NULL,
    event_class_id integer NOT NULL,
    CONSTRAINT pk_event PRIMARY KEY (event_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE contact (
    contact_id serial NOT NULL,
    contact_value varchar(50),
    club_member_id integer NOT NULL,
    contact_type_id integer NOT NULL,
    CONSTRAINT pk_contact PRIMARY KEY (contact_id, club_member_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE contact_type (
    contact_type_id serial NOT NULL,
    contact_type_name varchar(50) NOT NULL,
    CONSTRAINT pk_contact_type PRIMARY KEY (contact_type_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE event_type (
    event_type_id serial NOT NULL,
    event_type_name varchar(50) NOT NULL,
    CONSTRAINT pk_event_type PRIMARY KEY (event_type_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE event_class (
    event_class_id serial NOT NULL,
    event_class_name varchar(50) NOT NULL,
    CONSTRAINT pk_event_class PRIMARY KEY (event_class_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE achievement (
    achievement_id serial NOT NULL,
    achievement_award_date date,
    achievement_judge_full_name varchar(150),
    event_id integer NOT NULL,
    grade_id integer NOT NULL,
    dog_id integer NOT NULL,
    CONSTRAINT pk_achievement PRIMARY KEY (achievement_id, dog_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE grade (
    grade_id serial NOT NULL,
    grade_name varchar(50) NOT NULL,
    CONSTRAINT pk_grade PRIMARY KEY (grade_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE dog_club_member (
    dog_id integer NOT NULL,
    club_member_id integer NOT NULL,
    CONSTRAINT pk_dog_club_member PRIMARY KEY (dog_id, club_member_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE veterinary_examination_disease (
    veterinary_examination_id integer NOT NULL,
    dog_id integer NOT NULL,
    disease_id integer NOT NULL,
    CONSTRAINT pk_veterinary_examination_disease PRIMARY KEY (veterinary_examination_id, dog_id, disease_id)
) WITH (autovacuum_enabled = true);

CREATE TABLE title_dog (
    title_id integer NOT NULL,
    dog_id integer NOT NULL,
    CONSTRAINT pk_title_dog PRIMARY KEY (title_id, dog_id)
) WITH (autovacuum_enabled = true);

ALTER TABLE pedigree ADD CONSTRAINT fk_pedigree_dog_id
    FOREIGN KEY (dog_id) REFERENCES dog (dog_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE vaccination ADD CONSTRAINT fk_vaccination_dog_id
    FOREIGN KEY (dog_id) REFERENCES dog (dog_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE dog ADD CONSTRAINT fk_dog_dog_status_id
    FOREIGN KEY (dog_status_id) REFERENCES dog_status (dog_status_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE dog_training_session ADD CONSTRAINT fk_dog_training_session_dog_id
    FOREIGN KEY (dog_id) REFERENCES dog (dog_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE vaccination ADD CONSTRAINT fk_vaccination_vaccine_type_id
    FOREIGN KEY (vaccine_type_id) REFERENCES vaccine_type (vaccine_type_id)
    ON DELETE NO ACTION ON UPDATE NO ACTION;

ALTER TABLE dog ADD CONSTRAINT fk_dog_identifier_type_id
    FOREIGN KEY (identifier_type_id) REFERENCES identifier_type (identifier_type_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE dog_training_session ADD CONSTRAINT fk_dog_training_session_club_member_id
    FOREIGN KEY (club_member_id) REFERENCES club_member (club_member_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE club_member ADD CONSTRAINT fk_club_member_role_id
    FOREIGN KEY (role_id) REFERENCES role (role_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE club_member ADD CONSTRAINT fk_club_member_membership_status_id
    FOREIGN KEY (membership_status_id) REFERENCES membership_status (membership_status_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE address ADD CONSTRAINT fk_address_club_member_id
    FOREIGN KEY (club_member_id) REFERENCES club_member (club_member_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE address ADD CONSTRAINT fk_address_address_type_id
    FOREIGN KEY (address_type_id) REFERENCES address_type (address_type_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE event ADD CONSTRAINT fk_event_address_id
    FOREIGN KEY (address_id) REFERENCES address (address_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE event ADD CONSTRAINT fk_event_event_type_id
    FOREIGN KEY (event_type_id) REFERENCES event_type (event_type_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE event ADD CONSTRAINT fk_event_event_class_id
    FOREIGN KEY (event_class_id) REFERENCES event_class (event_class_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE achievement ADD CONSTRAINT fk_achievement_event_id
    FOREIGN KEY (event_id) REFERENCES event (event_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE achievement ADD CONSTRAINT fk_achievement_grade_id
    FOREIGN KEY (grade_id) REFERENCES grade (grade_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE achievement ADD CONSTRAINT fk_achievement_dog_id
    FOREIGN KEY (dog_id) REFERENCES dog (dog_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE document ADD CONSTRAINT fk_document_dog_id
    FOREIGN KEY (dog_id) REFERENCES dog (dog_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE document ADD CONSTRAINT fk_document_club_member_id
    FOREIGN KEY (club_member_id) REFERENCES club_member (club_member_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE document ADD CONSTRAINT fk_document_document_type_id
    FOREIGN KEY (document_type_id) REFERENCES document_type (document_type_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE veterinary_examination ADD CONSTRAINT fk_veterinary_examination_dog_id
    FOREIGN KEY (dog_id) REFERENCES dog (dog_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE veterinary_examination ADD CONSTRAINT fk_veterinary_examination_club_member_id
    FOREIGN KEY (club_member_id) REFERENCES club_member (club_member_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE breeding ADD CONSTRAINT fk_breeding_female_dog_id
    FOREIGN KEY (female_dog_id) REFERENCES dog (dog_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE breeding ADD CONSTRAINT fk_breeding_male_dog_id
    FOREIGN KEY (male_dog_id) REFERENCES dog (dog_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE breeding ADD CONSTRAINT fk_breeding_breeding_status_id
    FOREIGN KEY (breeding_status_id) REFERENCES breeding_status (breeding_status_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE dog ADD CONSTRAINT fk_dog_dog_breed_id
    FOREIGN KEY (dog_breed_id) REFERENCES dog_breed (dog_breed_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE dog ADD CONSTRAINT fk_dog_coat_color_id
    FOREIGN KEY (coat_color_id) REFERENCES coat_color (coat_color_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE dog ADD CONSTRAINT fk_dog_sex_id
    FOREIGN KEY (sex_id) REFERENCES sex (sex_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE contact ADD CONSTRAINT fk_contact_club_member_id
    FOREIGN KEY (club_member_id) REFERENCES club_member (club_member_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE contact ADD CONSTRAINT fk_contact_contact_type_id
    FOREIGN KEY (contact_type_id) REFERENCES contact_type (contact_type_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE dog_training_session ADD CONSTRAINT fk_dog_training_session_training_type_id
    FOREIGN KEY (training_type_id) REFERENCES training_type (training_type_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE dog_club_member ADD CONSTRAINT fk_dog_club_member_dog_id
    FOREIGN KEY (dog_id) REFERENCES dog (dog_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE dog_club_member ADD CONSTRAINT fk_dog_club_member_club_member_id
    FOREIGN KEY (club_member_id) REFERENCES club_member (club_member_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE veterinary_examination_disease ADD CONSTRAINT fk_veterinary_examination_disease_examination
    FOREIGN KEY (veterinary_examination_id, dog_id) REFERENCES veterinary_examination (veterinary_examination_id, dog_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

ALTER TABLE veterinary_examination_disease ADD CONSTRAINT fk_veterinary_examination_disease_disease_id
    FOREIGN KEY (disease_id) REFERENCES disease (disease_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE title_dog ADD CONSTRAINT fk_title_dog_title_id
    FOREIGN KEY (title_id) REFERENCES title (title_id)
    ON DELETE RESTRICT ON UPDATE RESTRICT;

ALTER TABLE title_dog ADD CONSTRAINT fk_title_dog_dog_id
    FOREIGN KEY (dog_id) REFERENCES dog (dog_id)
    ON DELETE CASCADE ON UPDATE RESTRICT;

CREATE INDEX idx_dog_dog_status_id ON dog (dog_status_id);
CREATE INDEX idx_dog_identifier_type_id ON dog (identifier_type_id);
CREATE INDEX idx_dog_dog_breed_id ON dog (dog_breed_id);
CREATE INDEX idx_dog_coat_color_id ON dog (coat_color_id);
CREATE INDEX idx_dog_sex_id ON dog (sex_id);
CREATE INDEX idx_club_member_role_id ON club_member (role_id);
CREATE INDEX idx_club_member_membership_status_id ON club_member (membership_status_id);
CREATE INDEX idx_document_dog_id ON document (dog_id);
CREATE INDEX idx_document_club_member_id ON document (club_member_id);
CREATE INDEX idx_document_document_type_id ON document (document_type_id);
CREATE INDEX idx_veterinary_examination_club_member_id ON veterinary_examination (club_member_id);
CREATE INDEX idx_vaccination_vaccine_type_id ON vaccination (vaccine_type_id);
CREATE INDEX idx_breeding_breeding_status_id ON breeding (breeding_status_id);
CREATE INDEX idx_dog_training_session_club_member_id ON dog_training_session (club_member_id);
CREATE INDEX idx_dog_training_session_training_type_id ON dog_training_session (training_type_id);
CREATE INDEX idx_address_club_member_id ON address (club_member_id);
CREATE INDEX idx_address_address_type_id ON address (address_type_id);
CREATE INDEX idx_event_address_id ON event (address_id);
CREATE INDEX idx_event_event_type_id ON event (event_type_id);
CREATE INDEX idx_event_event_class_id ON event (event_class_id);
CREATE INDEX idx_contact_contact_type_id ON contact (contact_type_id);
CREATE INDEX idx_achievement_event_id ON achievement (event_id);
CREATE INDEX idx_achievement_grade_id ON achievement (grade_id);
CREATE INDEX idx_dog_club_member_club_member_id ON dog_club_member (club_member_id);
CREATE INDEX idx_veterinary_examination_disease_disease_id ON veterinary_examination_disease (disease_id);
CREATE INDEX idx_title_dog_dog_id ON title_dog (dog_id);

-- +goose Down
-- Destructive rollback: use only on a disposable database.
DROP TABLE achievement;
DROP TABLE veterinary_examination_disease;
DROP TABLE event;
DROP TABLE veterinary_examination;
DROP TABLE vaccination;
DROP TABLE title_dog;
DROP TABLE pedigree;
DROP TABLE dog_training_session;
DROP TABLE dog_club_member;
DROP TABLE document;
DROP TABLE contact;
DROP TABLE breeding;
DROP TABLE address;
DROP TABLE dog;
DROP TABLE club_member;
DROP TABLE vaccine_type;
DROP TABLE training_type;
DROP TABLE title;
DROP TABLE sex;
DROP TABLE role;
DROP TABLE membership_status;
DROP TABLE identifier_type;
DROP TABLE grade;
DROP TABLE event_type;
DROP TABLE event_class;
DROP TABLE dog_status;
DROP TABLE dog_breed;
DROP TABLE document_type;
DROP TABLE disease;
DROP TABLE contact_type;
DROP TABLE coat_color;
DROP TABLE breeding_status;
DROP TABLE address_type;
