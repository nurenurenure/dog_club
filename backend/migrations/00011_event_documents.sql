-- +goose Up
INSERT INTO document_type(document_type_name) VALUES('Список участников мероприятия'),('Протокол мероприятия');
ALTER TABLE achievement ADD CONSTRAINT uq_achievement_document_basis UNIQUE(achievement_id,dog_id,event_id);
ALTER TABLE document ADD COLUMN event_id integer REFERENCES event(event_id) ON DELETE RESTRICT,
 ADD COLUMN event_result_id integer,
 ADD CONSTRAINT fk_document_event_result FOREIGN KEY(event_result_id,dog_id,event_id) REFERENCES achievement(achievement_id,dog_id,event_id) ON DELETE RESTRICT,
 DROP CONSTRAINT ck_document_subject,
 ADD CONSTRAINT ck_document_subject CHECK(status='legacy' OR
 (event_id IS NULL AND ((dog_id IS NOT NULL)::integer+(subject_member_id IS NOT NULL)::integer=1)) OR
 (event_id IS NOT NULL AND subject_member_id IS NULL AND
 ((dog_id IS NULL AND event_result_id IS NULL AND recipient_member_id IS NULL) OR
 (dog_id IS NOT NULL AND event_result_id IS NOT NULL AND recipient_member_id IS NOT NULL)))),
 ADD CONSTRAINT ck_document_event_basis CHECK(
 (event_id IS NULL AND event_result_id IS NULL) OR
 (event_id IS NOT NULL AND category IS NOT DISTINCT FROM 'general' AND breeding_decision_id IS NULL AND litter_id IS NULL
 AND subject_member_id IS NULL AND ((dog_id IS NULL AND event_result_id IS NULL AND recipient_member_id IS NULL) OR
 (dog_id IS NOT NULL AND event_result_id IS NOT NULL AND recipient_member_id IS NOT NULL))));
CREATE INDEX idx_document_event ON document(event_id,document_id);
CREATE INDEX idx_document_event_result ON document(event_result_id);
ALTER TABLE document_file DROP CONSTRAINT document_file_template_check,
 ADD CONSTRAINT document_file_template_check CHECK(template IN ('member_card','dog_card','veterinary_conclusion','registration_certificate','breeding_conclusion','litter_act','event_participants','event_protocol','event_diploma'));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM document WHERE event_id IS NOT NULL OR event_result_id IS NOT NULL)
 OR EXISTS(SELECT 1 FROM document_file WHERE template IN ('event_participants','event_protocol','event_diploma')) THEN
 RAISE EXCEPTION 'event documents rollback requires no new information';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE document_file DROP CONSTRAINT document_file_template_check,
 ADD CONSTRAINT document_file_template_check CHECK(template IN ('member_card','dog_card','veterinary_conclusion','registration_certificate','breeding_conclusion','litter_act'));
ALTER TABLE document DROP CONSTRAINT ck_document_event_basis,DROP CONSTRAINT ck_document_subject,
 DROP COLUMN event_result_id,DROP COLUMN event_id,
 ADD CONSTRAINT ck_document_subject CHECK(status='legacy' OR ((dog_id IS NOT NULL)::integer+(subject_member_id IS NOT NULL)::integer=1));
ALTER TABLE achievement DROP CONSTRAINT uq_achievement_document_basis;
DELETE FROM document_type WHERE document_type_name IN ('Список участников мероприятия','Протокол мероприятия');
