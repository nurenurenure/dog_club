-- +goose Up
ALTER TABLE dog ADD COLUMN registration_date date,
 ADD CONSTRAINT ck_dog_registration_date CHECK(registration_date IS NULL OR registration_date>=dog_date_of_birth::date);
-- Registration dates remain unknown for existing dogs; do not derive from audits.
INSERT INTO document_type(document_type_name) VALUES('Заключение о вязке'),('Акт о регистрации помёта');
ALTER TABLE document DROP CONSTRAINT document_category_check,
 ADD CONSTRAINT document_category_check CHECK(category IN ('general','medical','breeding')),
 ADD COLUMN breeding_decision_id bigint REFERENCES breeding_decision(decision_id) ON DELETE RESTRICT,
 ADD COLUMN litter_id bigint REFERENCES litter(litter_id) ON DELETE RESTRICT,
 ADD CONSTRAINT ck_document_breeding_basis CHECK((breeding_decision_id IS NULL OR litter_id IS NULL) AND
 ((category='breeding' AND dog_id IS NOT NULL AND ((breeding_decision_id IS NOT NULL)::integer+(litter_id IS NOT NULL)::integer=1)) OR
 (category IS DISTINCT FROM 'breeding' AND breeding_decision_id IS NULL AND litter_id IS NULL)));
CREATE INDEX idx_document_breeding_decision ON document(breeding_decision_id);
CREATE INDEX idx_document_litter ON document(litter_id);
ALTER TABLE document_file DROP CONSTRAINT document_file_template_check,
 ADD CONSTRAINT document_file_template_check CHECK(template IN ('member_card','dog_card','veterinary_conclusion','registration_certificate','breeding_conclusion','litter_act'));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM dog WHERE registration_date IS NOT NULL)
 OR EXISTS(SELECT 1 FROM dog_changes WHERE before_data ? 'registration_date' OR after_data ? 'registration_date')
 OR EXISTS(SELECT 1 FROM document WHERE category='breeding' OR breeding_decision_id IS NOT NULL OR litter_id IS NOT NULL)
 OR EXISTS(SELECT 1 FROM document_file WHERE template IN ('registration_certificate','breeding_conclusion','litter_act')) THEN
 RAISE EXCEPTION 'registration and breeding documents rollback requires no new information';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE document_file DROP CONSTRAINT document_file_template_check,
 ADD CONSTRAINT document_file_template_check CHECK(template IN ('member_card','dog_card','veterinary_conclusion'));
ALTER TABLE document DROP CONSTRAINT ck_document_breeding_basis,DROP COLUMN breeding_decision_id,DROP COLUMN litter_id,
 DROP CONSTRAINT document_category_check,ADD CONSTRAINT document_category_check CHECK(category IN ('general','medical'));
DELETE FROM document_type WHERE document_type_name IN ('Заключение о вязке','Акт о регистрации помёта');
ALTER TABLE dog DROP CONSTRAINT ck_dog_registration_date,DROP COLUMN registration_date;
