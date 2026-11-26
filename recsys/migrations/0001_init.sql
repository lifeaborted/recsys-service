--
-- PostgreSQL database dump
--

-- Dumped from database version 17.6
-- Dumped by pg_dump version 17.6

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: recommendations; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA IF NOT EXISTS "recommendations";

DO $bootstrap$
BEGIN
IF to_regclass('recommendations.sales') IS NULL THEN

SET default_tablespace = '';

SET default_table_access_method = "heap";

--
-- Name: account_memberships; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."account_memberships" (
    "client_id" "text" NOT NULL,
    "user_id" bigint NOT NULL,
    "role" "text" NOT NULL,
    CONSTRAINT "account_memberships_role_check" CHECK (("role" = ANY (ARRAY['owner'::"text", 'admin'::"text", 'buyer'::"text", 'viewer'::"text"])))
);

--
-- Name: account_profiles; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."account_profiles" (
    "client_id" "text" NOT NULL,
    "company_name" "text",
    "tax_id" "text",
    "registration_code" "text",
    "legal_address" "text",
    "actual_address" "text"
);

--
-- Name: account_users; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."account_users" (
    "user_id" bigint NOT NULL,
    "email" "text" NOT NULL,
    "display_name" "text",
    "phone" "text",
    "created_at" timestamp with time zone DEFAULT "now"() NOT NULL,
    CONSTRAINT "account_users_email_check" CHECK ((("email" = "lower"("btrim"("email"))) AND ("email" <> ''::"text")))
);

--
-- Name: account_users_user_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."account_users" ALTER COLUMN "user_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."account_users_user_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: cart_items; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."cart_items" (
    "cart_id" bigint NOT NULL,
    "offer_id" "text" NOT NULL,
    "quantity" integer NOT NULL,
    CONSTRAINT "cart_items_quantity_check" CHECK (("quantity" > 0))
);

--
-- Name: carts; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."carts" (
    "cart_id" bigint NOT NULL,
    "client_id" "text" NOT NULL,
    "user_id" bigint NOT NULL,
    "status" "text" DEFAULT 'open'::"text" NOT NULL,
    "created_at" timestamp with time zone DEFAULT "now"() NOT NULL,
    "order_id" "text",
    CONSTRAINT "cart_order_status_check" CHECK ((("status" = 'ordered'::"text") = ("order_id" IS NOT NULL))),
    CONSTRAINT "carts_status_check" CHECK (("status" = ANY (ARRAY['open'::"text", 'ordered'::"text", 'abandoned'::"text"])))
);

--
-- Name: carts_cart_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."carts" ALTER COLUMN "cart_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."carts_cart_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: catalog_categories; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."catalog_categories" (
    "category_id" "text" NOT NULL,
    "parent_id" "text",
    "name" "text" NOT NULL
);

--
-- Name: catalog_currencies; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."catalog_currencies" (
    "currency_code" "text" NOT NULL,
    "rate" numeric NOT NULL,
    CONSTRAINT "catalog_currencies_rate_check" CHECK (("rate" > (0)::numeric))
);

--
-- Name: catalog_groups; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."catalog_groups" (
    "group_id" "text" NOT NULL
);

--
-- Name: catalog_notes; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."catalog_notes" (
    "excel_row" integer NOT NULL,
    "cell_a" "text"
);

--
-- Name: TABLE "catalog_notes"; Type: COMMENT; Schema: recommendations; Owner: -
--

COMMENT ON TABLE "recommendations"."catalog_notes" IS 'Excel: Каталог';

--
-- Name: catalog_offers; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."catalog_offers" (
    "offer_id" "text" NOT NULL,
    "snapshot_id" bigint NOT NULL,
    "category_id" "text" NOT NULL,
    "vendor_id" bigint,
    "group_id" "text",
    "name" "text" NOT NULL,
    "vendor_code" "text",
    "vendor_url" "text",
    "url" "text" NOT NULL,
    "description" "text",
    "picture_url" "text",
    "available" boolean NOT NULL,
    "price" numeric,
    "old_price" numeric,
    "currency_code" "text",
    "delivery" boolean,
    "pickup" boolean,
    "sales_notes" "text",
    "parameters" "jsonb" NOT NULL,
    "vendor_code_key" "text",
    "url_code_key" "text",
    "name_key" "text",
    CONSTRAINT "catalog_offers_old_price_check" CHECK (("old_price" >= (0)::numeric)),
    CONSTRAINT "catalog_offers_parameters_check" CHECK (("jsonb_typeof"("parameters") = 'array'::"text")),
    CONSTRAINT "catalog_offers_price_check" CHECK (("price" >= (0)::numeric))
);

--
-- Name: catalog_snapshots; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."catalog_snapshots" (
    "id" bigint NOT NULL,
    "source_url" "text" NOT NULL,
    "feed_generated_at" timestamp without time zone NOT NULL,
    "imported_at" timestamp with time zone DEFAULT "now"() NOT NULL
);

--
-- Name: catalog_snapshots_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."catalog_snapshots" ALTER COLUMN "id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."catalog_snapshots_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: catalog_vendors; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."catalog_vendors" (
    "vendor_id" bigint NOT NULL,
    "name" "text" NOT NULL
);

--
-- Name: catalog_vendors_vendor_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."catalog_vendors" ALTER COLUMN "vendor_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."catalog_vendors_vendor_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: clients; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."clients" (
    "excel_row" integer NOT NULL,
    "client_id" "text" NOT NULL,
    "client_group" "text",
    "client_type" "text",
    "industry" "text",
    "segment" "text",
    "lk_access" "text"
);

--
-- Name: TABLE "clients"; Type: COMMENT; Schema: recommendations; Owner: -
--

COMMENT ON TABLE "recommendations"."clients" IS 'Excel: Клиенты';

--
-- Name: current_block; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."current_block" (
    "excel_row" integer NOT NULL,
    "cell_a" "text",
    "cell_b" "text",
    "cell_c" "text"
);

--
-- Name: TABLE "current_block"; Type: COMMENT; Schema: recommendations; Owner: -
--

COMMENT ON TABLE "recommendations"."current_block" IS 'Excel: Текущий_блок';

--
-- Name: historical_item_offers; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."historical_item_offers" (
    "item_id" "text" NOT NULL,
    "offer_id" "text" NOT NULL,
    "match_method" "text" NOT NULL,
    "is_selected" boolean NOT NULL,
    CONSTRAINT "historical_item_offers_match_method_check" CHECK (("match_method" = ANY (ARRAY['vendor_code'::"text", 'url_code'::"text", 'name'::"text"])))
);

--
-- Name: COLUMN "historical_item_offers"."is_selected"; Type: COMMENT; Schema: recommendations; Owner: -
--

COMMENT ON COLUMN "recommendations"."historical_item_offers"."is_selected" IS 'True only when exactly one offer has the strongest evidence for the item.';

--
-- Name: historical_items; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."historical_items" (
    "item_id" "text" NOT NULL
);

--
-- Name: licenses; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."licenses" (
    "license_id" bigint NOT NULL,
    "order_id" "text" NOT NULL,
    "order_item_id" bigint,
    "offer_id" "text" NOT NULL,
    "status" "text" NOT NULL,
    "issued_at" timestamp with time zone,
    "expires_at" timestamp with time zone,
    "key_ref" "text",
    "download_ref" "text",
    CONSTRAINT "licenses_check" CHECK ((("expires_at" IS NULL) OR ("issued_at" IS NULL) OR ("expires_at" >= "issued_at")))
);

--
-- Name: licenses_license_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."licenses" ALTER COLUMN "license_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."licenses_license_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: order_deliveries; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."order_deliveries" (
    "delivery_id" bigint NOT NULL,
    "order_id" "text" NOT NULL,
    "method" "text" NOT NULL,
    "status" "text" NOT NULL,
    "recipient_address" "text",
    "tracking_reference" "text",
    "fulfilled_at" timestamp with time zone,
    CONSTRAINT "order_deliveries_method_check" CHECK (("method" = ANY (ARRAY['digital'::"text", 'courier'::"text", 'pickup'::"text"])))
);

--
-- Name: order_deliveries_delivery_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."order_deliveries" ALTER COLUMN "delivery_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."order_deliveries_delivery_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: order_documents; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."order_documents" (
    "document_id" bigint NOT NULL,
    "order_id" "text" NOT NULL,
    "document_type" "text" NOT NULL,
    "document_number" "text",
    "issued_at" timestamp with time zone,
    "storage_ref" "text"
);

--
-- Name: order_documents_document_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."order_documents" ALTER COLUMN "document_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."order_documents_document_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: order_invoices; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."order_invoices" (
    "invoice_id" bigint NOT NULL,
    "order_id" "text" NOT NULL,
    "invoice_number" "text" NOT NULL,
    "status" "text" NOT NULL,
    "amount" numeric,
    "currency_code" "text",
    "issued_at" timestamp with time zone,
    "paid_at" timestamp with time zone,
    CONSTRAINT "order_invoices_amount_check" CHECK (("amount" >= (0)::numeric))
);

--
-- Name: order_invoices_invoice_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."order_invoices" ALTER COLUMN "invoice_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."order_invoices_invoice_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: order_observed_weeks; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."order_observed_weeks" (
    "order_id" "text" NOT NULL,
    "order_week" "date" NOT NULL
);

--
-- Name: order_payments; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."order_payments" (
    "payment_id" bigint NOT NULL,
    "order_id" "text" NOT NULL,
    "method" "text" NOT NULL,
    "status" "text" NOT NULL,
    "amount" numeric NOT NULL,
    "currency_code" "text" NOT NULL,
    "provider_reference" "text",
    "recorded_at" timestamp with time zone DEFAULT "now"() NOT NULL,
    CONSTRAINT "order_payments_amount_check" CHECK (("amount" >= (0)::numeric))
);

--
-- Name: order_payments_payment_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."order_payments" ALTER COLUMN "payment_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."order_payments_payment_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: orders; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."orders" (
    "order_id" "text" NOT NULL,
    "client_id" "text" NOT NULL,
    "source" "text" DEFAULT 'store'::"text" NOT NULL,
    "order_week" "date",
    "created_at" timestamp with time zone,
    "status" "text",
    CONSTRAINT "orders_source_check" CHECK (("source" = ANY (ARRAY['history'::"text", 'store'::"text", 'account'::"text"])))
);

--
-- Name: COLUMN "orders"."order_week"; Type: COMMENT; Schema: recommendations; Owner: -
--

COMMENT ON COLUMN "recommendations"."orders"."order_week" IS 'NULL if the dataset lists multiple weeks. See order_observed_weeks.';

--
-- Name: sales; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."sales" (
    "excel_row" integer NOT NULL,
    "order_id" "text" NOT NULL,
    "client_id" "text" NOT NULL,
    "order_week" "date" NOT NULL,
    "item_id" "text" NOT NULL,
    "sku" "text",
    "item_name" "text"
);

--
-- Name: TABLE "sales"; Type: COMMENT; Schema: recommendations; Owner: -
--

COMMENT ON TABLE "recommendations"."sales" IS 'Excel: Продажи';

--
-- Name: purchase_history; Type: VIEW; Schema: recommendations; Owner: -
--

CREATE VIEW "recommendations"."purchase_history" AS
 SELECT "s"."excel_row",
    "s"."order_id",
    "s"."client_id",
    "s"."order_week",
    "s"."item_id",
    "s"."sku",
    "s"."item_name",
    "matched"."offer_id"
   FROM ("recommendations"."sales" "s"
     LEFT JOIN "recommendations"."historical_item_offers" "matched" ON ((("matched"."item_id" = "s"."item_id") AND "matched"."is_selected")));

--
-- Name: readme; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."readme" (
    "excel_row" integer NOT NULL,
    "cell_a" "text",
    "cell_b" "text",
    "cell_c" "text"
);

--
-- Name: TABLE "readme"; Type: COMMENT; Schema: recommendations; Owner: -
--

COMMENT ON TABLE "recommendations"."readme" IS 'Excel: README';

--
-- Name: recommendation_events; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."recommendation_events" (
    "event_id" bigint NOT NULL,
    "run_id" bigint NOT NULL,
    "offer_id" "text" NOT NULL,
    "event_type" "text" NOT NULL,
    "occurred_at" timestamp with time zone DEFAULT "now"() NOT NULL,
    CONSTRAINT "recommendation_events_event_type_check" CHECK (("event_type" = ANY (ARRAY['click'::"text", 'purchase'::"text"])))
);

--
-- Name: recommendation_events_event_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."recommendation_events" ALTER COLUMN "event_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."recommendation_events_event_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: recommendation_items; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."recommendation_items" (
    "run_id" bigint NOT NULL,
    "rank" smallint NOT NULL,
    "offer_id" "text" NOT NULL,
    "source_item_id" "text",
    "explanation" "text" NOT NULL,
    "score" double precision,
    CONSTRAINT "recommendation_items_rank_check" CHECK ((("rank" >= 1) AND ("rank" <= 3)))
);

--
-- Name: recommendation_runs; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."recommendation_runs" (
    "run_id" bigint NOT NULL,
    "client_id" "text" NOT NULL,
    "model_version" "text" NOT NULL,
    "requested_at" timestamp with time zone DEFAULT "now"() NOT NULL,
    "shown_at" timestamp with time zone,
    CONSTRAINT "recommendation_runs_check" CHECK ((("shown_at" IS NULL) OR ("shown_at" >= "requested_at")))
);

--
-- Name: recommendation_runs_run_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."recommendation_runs" ALTER COLUMN "run_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."recommendation_runs_run_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: store_order_items; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."store_order_items" (
    "order_item_id" bigint NOT NULL,
    "order_id" "text" NOT NULL,
    "offer_id" "text" NOT NULL,
    "quantity" integer NOT NULL,
    "unit_price" numeric NOT NULL,
    "currency_code" "text" NOT NULL,
    CONSTRAINT "store_order_items_quantity_check" CHECK (("quantity" > 0)),
    CONSTRAINT "store_order_items_unit_price_check" CHECK (("unit_price" >= (0)::numeric))
);

--
-- Name: store_order_items_order_item_id_seq; Type: SEQUENCE; Schema: recommendations; Owner: -
--

ALTER TABLE "recommendations"."store_order_items" ALTER COLUMN "order_item_id" ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME "recommendations"."store_order_items_order_item_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: storefront_catalog; Type: VIEW; Schema: recommendations; Owner: -
--

CREATE VIEW "recommendations"."storefront_catalog" AS
 SELECT "o"."offer_id",
    "o"."group_id",
    "o"."name",
    "o"."vendor_code",
    "v"."name" AS "vendor",
    "o"."category_id",
    "c"."name" AS "category",
    "o"."url",
    "o"."picture_url",
    "o"."description",
    "o"."available",
    "o"."price",
    "o"."old_price",
    "o"."currency_code",
    "o"."delivery",
    "o"."pickup",
    "o"."sales_notes",
    "o"."parameters"
   FROM (("recommendations"."catalog_offers" "o"
     JOIN "recommendations"."catalog_categories" "c" USING ("category_id"))
     LEFT JOIN "recommendations"."catalog_vendors" "v" USING ("vendor_id"));

--
-- Name: validation_protocol; Type: TABLE; Schema: recommendations; Owner: -
--

CREATE TABLE "recommendations"."validation_protocol" (
    "excel_row" integer NOT NULL,
    "cell_a" "text",
    "cell_b" "text",
    "cell_c" "text"
);

--
-- Name: TABLE "validation_protocol"; Type: COMMENT; Schema: recommendations; Owner: -
--

COMMENT ON TABLE "recommendations"."validation_protocol" IS 'Excel: Протокол_проверки';

--
-- Data for Name: account_memberships; Type: TABLE DATA; Schema: recommendations; Owner: -
--

--
-- Name: account_users_user_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."account_users_user_id_seq"', 3, true);

--
-- Name: carts_cart_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."carts_cart_id_seq"', 1, true);

--
-- Name: catalog_snapshots_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."catalog_snapshots_id_seq"', 1, true);

--
-- Name: catalog_vendors_vendor_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."catalog_vendors_vendor_id_seq"', 1234, true);

--
-- Name: licenses_license_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."licenses_license_id_seq"', 2, true);

--
-- Name: order_deliveries_delivery_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."order_deliveries_delivery_id_seq"', 1, true);

--
-- Name: order_documents_document_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."order_documents_document_id_seq"', 1, true);

--
-- Name: order_invoices_invoice_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."order_invoices_invoice_id_seq"', 1, true);

--
-- Name: order_payments_payment_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."order_payments_payment_id_seq"', 1, true);

--
-- Name: recommendation_events_event_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."recommendation_events_event_id_seq"', 2, true);

--
-- Name: recommendation_runs_run_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."recommendation_runs_run_id_seq"', 1, true);

--
-- Name: store_order_items_order_item_id_seq; Type: SEQUENCE SET; Schema: recommendations; Owner: -
--

PERFORM pg_catalog.setval('"recommendations"."store_order_items_order_item_id_seq"', 1, true);

--
-- Name: account_memberships account_memberships_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."account_memberships"
    ADD CONSTRAINT "account_memberships_pkey" PRIMARY KEY ("client_id", "user_id");

--
-- Name: account_profiles account_profiles_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."account_profiles"
    ADD CONSTRAINT "account_profiles_pkey" PRIMARY KEY ("client_id");

--
-- Name: account_users account_users_email_key; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."account_users"
    ADD CONSTRAINT "account_users_email_key" UNIQUE ("email");

--
-- Name: account_users account_users_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."account_users"
    ADD CONSTRAINT "account_users_pkey" PRIMARY KEY ("user_id");

--
-- Name: cart_items cart_items_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."cart_items"
    ADD CONSTRAINT "cart_items_pkey" PRIMARY KEY ("cart_id", "offer_id");

--
-- Name: carts carts_order_id_key; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."carts"
    ADD CONSTRAINT "carts_order_id_key" UNIQUE ("order_id");

--
-- Name: carts carts_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."carts"
    ADD CONSTRAINT "carts_pkey" PRIMARY KEY ("cart_id");

--
-- Name: catalog_categories catalog_categories_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_categories"
    ADD CONSTRAINT "catalog_categories_pkey" PRIMARY KEY ("category_id");

--
-- Name: catalog_currencies catalog_currencies_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_currencies"
    ADD CONSTRAINT "catalog_currencies_pkey" PRIMARY KEY ("currency_code");

--
-- Name: catalog_groups catalog_groups_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_groups"
    ADD CONSTRAINT "catalog_groups_pkey" PRIMARY KEY ("group_id");

--
-- Name: catalog_notes catalog_notes_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_notes"
    ADD CONSTRAINT "catalog_notes_pkey" PRIMARY KEY ("excel_row");

--
-- Name: catalog_offers catalog_offers_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_offers"
    ADD CONSTRAINT "catalog_offers_pkey" PRIMARY KEY ("offer_id");

--
-- Name: catalog_snapshots catalog_snapshots_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_snapshots"
    ADD CONSTRAINT "catalog_snapshots_pkey" PRIMARY KEY ("id");

--
-- Name: catalog_snapshots catalog_snapshots_source_url_feed_generated_at_key; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_snapshots"
    ADD CONSTRAINT "catalog_snapshots_source_url_feed_generated_at_key" UNIQUE ("source_url", "feed_generated_at");

--
-- Name: catalog_vendors catalog_vendors_name_key; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_vendors"
    ADD CONSTRAINT "catalog_vendors_name_key" UNIQUE ("name");

--
-- Name: catalog_vendors catalog_vendors_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_vendors"
    ADD CONSTRAINT "catalog_vendors_pkey" PRIMARY KEY ("vendor_id");

--
-- Name: clients clients_client_id_key; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."clients"
    ADD CONSTRAINT "clients_client_id_key" UNIQUE ("client_id");

--
-- Name: clients clients_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."clients"
    ADD CONSTRAINT "clients_pkey" PRIMARY KEY ("excel_row");

--
-- Name: current_block current_block_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."current_block"
    ADD CONSTRAINT "current_block_pkey" PRIMARY KEY ("excel_row");

--
-- Name: historical_item_offers historical_item_offers_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."historical_item_offers"
    ADD CONSTRAINT "historical_item_offers_pkey" PRIMARY KEY ("item_id", "offer_id");

--
-- Name: historical_items historical_items_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."historical_items"
    ADD CONSTRAINT "historical_items_pkey" PRIMARY KEY ("item_id");

--
-- Name: licenses licenses_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."licenses"
    ADD CONSTRAINT "licenses_pkey" PRIMARY KEY ("license_id");

--
-- Name: order_deliveries order_deliveries_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_deliveries"
    ADD CONSTRAINT "order_deliveries_pkey" PRIMARY KEY ("delivery_id");

--
-- Name: order_documents order_documents_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_documents"
    ADD CONSTRAINT "order_documents_pkey" PRIMARY KEY ("document_id");

--
-- Name: order_invoices order_invoices_order_id_invoice_number_key; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_invoices"
    ADD CONSTRAINT "order_invoices_order_id_invoice_number_key" UNIQUE ("order_id", "invoice_number");

--
-- Name: order_invoices order_invoices_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_invoices"
    ADD CONSTRAINT "order_invoices_pkey" PRIMARY KEY ("invoice_id");

--
-- Name: order_observed_weeks order_observed_weeks_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_observed_weeks"
    ADD CONSTRAINT "order_observed_weeks_pkey" PRIMARY KEY ("order_id", "order_week");

--
-- Name: order_payments order_payments_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_payments"
    ADD CONSTRAINT "order_payments_pkey" PRIMARY KEY ("payment_id");

--
-- Name: orders orders_order_id_client_id_key; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."orders"
    ADD CONSTRAINT "orders_order_id_client_id_key" UNIQUE ("order_id", "client_id");

--
-- Name: orders orders_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."orders"
    ADD CONSTRAINT "orders_pkey" PRIMARY KEY ("order_id");

--
-- Name: readme readme_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."readme"
    ADD CONSTRAINT "readme_pkey" PRIMARY KEY ("excel_row");

--
-- Name: recommendation_events recommendation_events_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."recommendation_events"
    ADD CONSTRAINT "recommendation_events_pkey" PRIMARY KEY ("event_id");

--
-- Name: recommendation_items recommendation_items_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."recommendation_items"
    ADD CONSTRAINT "recommendation_items_pkey" PRIMARY KEY ("run_id", "rank");

--
-- Name: recommendation_items recommendation_items_run_id_offer_id_key; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."recommendation_items"
    ADD CONSTRAINT "recommendation_items_run_id_offer_id_key" UNIQUE ("run_id", "offer_id");

--
-- Name: recommendation_runs recommendation_runs_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."recommendation_runs"
    ADD CONSTRAINT "recommendation_runs_pkey" PRIMARY KEY ("run_id");

--
-- Name: sales sales_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."sales"
    ADD CONSTRAINT "sales_pkey" PRIMARY KEY ("excel_row");

--
-- Name: store_order_items store_order_items_order_item_id_order_id_key; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."store_order_items"
    ADD CONSTRAINT "store_order_items_order_item_id_order_id_key" UNIQUE ("order_item_id", "order_id");

--
-- Name: store_order_items store_order_items_order_item_id_order_id_offer_id_key; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."store_order_items"
    ADD CONSTRAINT "store_order_items_order_item_id_order_id_offer_id_key" UNIQUE ("order_item_id", "order_id", "offer_id");

--
-- Name: store_order_items store_order_items_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."store_order_items"
    ADD CONSTRAINT "store_order_items_pkey" PRIMARY KEY ("order_item_id");

--
-- Name: validation_protocol validation_protocol_pkey; Type: CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."validation_protocol"
    ADD CONSTRAINT "validation_protocol_pkey" PRIMARY KEY ("excel_row");

--
-- Name: account_memberships_user_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "account_memberships_user_id_idx" ON "recommendations"."account_memberships" USING "btree" ("user_id");

--
-- Name: carts_client_id_user_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE UNIQUE INDEX "carts_client_id_user_id_idx" ON "recommendations"."carts" USING "btree" ("client_id", "user_id") WHERE ("status" = 'open'::"text");

--
-- Name: catalog_categories_parent_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "catalog_categories_parent_id_idx" ON "recommendations"."catalog_categories" USING "btree" ("parent_id");

--
-- Name: catalog_offers_category_id_available_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "catalog_offers_category_id_available_idx" ON "recommendations"."catalog_offers" USING "btree" ("category_id", "available");

--
-- Name: catalog_offers_group_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "catalog_offers_group_id_idx" ON "recommendations"."catalog_offers" USING "btree" ("group_id");

--
-- Name: catalog_offers_name_key_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "catalog_offers_name_key_idx" ON "recommendations"."catalog_offers" USING "btree" ("name_key");

--
-- Name: catalog_offers_url_code_key_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "catalog_offers_url_code_key_idx" ON "recommendations"."catalog_offers" USING "btree" ("url_code_key");

--
-- Name: catalog_offers_vendor_code_key_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "catalog_offers_vendor_code_key_idx" ON "recommendations"."catalog_offers" USING "btree" ("vendor_code_key");

--
-- Name: catalog_offers_vendor_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "catalog_offers_vendor_id_idx" ON "recommendations"."catalog_offers" USING "btree" ("vendor_id");

--
-- Name: historical_item_offers_item_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE UNIQUE INDEX "historical_item_offers_item_id_idx" ON "recommendations"."historical_item_offers" USING "btree" ("item_id") WHERE "is_selected";

--
-- Name: historical_item_offers_offer_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "historical_item_offers_offer_id_idx" ON "recommendations"."historical_item_offers" USING "btree" ("offer_id");

--
-- Name: licenses_order_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "licenses_order_id_idx" ON "recommendations"."licenses" USING "btree" ("order_id");

--
-- Name: order_deliveries_order_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "order_deliveries_order_id_idx" ON "recommendations"."order_deliveries" USING "btree" ("order_id");

--
-- Name: order_documents_order_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "order_documents_order_id_idx" ON "recommendations"."order_documents" USING "btree" ("order_id");

--
-- Name: order_payments_order_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "order_payments_order_id_idx" ON "recommendations"."order_payments" USING "btree" ("order_id");

--
-- Name: orders_client_id_order_week_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "orders_client_id_order_week_idx" ON "recommendations"."orders" USING "btree" ("client_id", "order_week" DESC);

--
-- Name: recommendation_events_run_id_offer_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "recommendation_events_run_id_offer_id_idx" ON "recommendations"."recommendation_events" USING "btree" ("run_id", "offer_id");

--
-- Name: recommendation_runs_client_id_requested_at_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "recommendation_runs_client_id_requested_at_idx" ON "recommendations"."recommendation_runs" USING "btree" ("client_id", "requested_at" DESC);

--
-- Name: sales_client_id_order_week_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "sales_client_id_order_week_idx" ON "recommendations"."sales" USING "btree" ("client_id", "order_week");

--
-- Name: sales_item_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "sales_item_id_idx" ON "recommendations"."sales" USING "btree" ("item_id");

--
-- Name: sales_order_id_order_week_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "sales_order_id_order_week_idx" ON "recommendations"."sales" USING "btree" ("order_id", "order_week");

--
-- Name: sales_sku_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "sales_sku_idx" ON "recommendations"."sales" USING "btree" ("sku");

--
-- Name: store_order_items_order_id_idx; Type: INDEX; Schema: recommendations; Owner: -
--

CREATE INDEX "store_order_items_order_id_idx" ON "recommendations"."store_order_items" USING "btree" ("order_id");

--
-- Name: account_memberships account_memberships_client_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."account_memberships"
    ADD CONSTRAINT "account_memberships_client_id_fkey" FOREIGN KEY ("client_id") REFERENCES "recommendations"."clients"("client_id");

--
-- Name: account_memberships account_memberships_user_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."account_memberships"
    ADD CONSTRAINT "account_memberships_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "recommendations"."account_users"("user_id");

--
-- Name: account_profiles account_profiles_client_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."account_profiles"
    ADD CONSTRAINT "account_profiles_client_id_fkey" FOREIGN KEY ("client_id") REFERENCES "recommendations"."clients"("client_id");

--
-- Name: cart_items cart_items_cart_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."cart_items"
    ADD CONSTRAINT "cart_items_cart_id_fkey" FOREIGN KEY ("cart_id") REFERENCES "recommendations"."carts"("cart_id") ON DELETE CASCADE;

--
-- Name: cart_items cart_items_offer_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."cart_items"
    ADD CONSTRAINT "cart_items_offer_id_fkey" FOREIGN KEY ("offer_id") REFERENCES "recommendations"."catalog_offers"("offer_id");

--
-- Name: carts cart_order_owner_fk; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."carts"
    ADD CONSTRAINT "cart_order_owner_fk" FOREIGN KEY ("order_id", "client_id") REFERENCES "recommendations"."orders"("order_id", "client_id");

--
-- Name: carts carts_client_id_user_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."carts"
    ADD CONSTRAINT "carts_client_id_user_id_fkey" FOREIGN KEY ("client_id", "user_id") REFERENCES "recommendations"."account_memberships"("client_id", "user_id");

--
-- Name: catalog_categories catalog_categories_parent_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_categories"
    ADD CONSTRAINT "catalog_categories_parent_id_fkey" FOREIGN KEY ("parent_id") REFERENCES "recommendations"."catalog_categories"("category_id") DEFERRABLE INITIALLY DEFERRED;

--
-- Name: catalog_offers catalog_offers_category_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_offers"
    ADD CONSTRAINT "catalog_offers_category_id_fkey" FOREIGN KEY ("category_id") REFERENCES "recommendations"."catalog_categories"("category_id");

--
-- Name: catalog_offers catalog_offers_currency_code_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_offers"
    ADD CONSTRAINT "catalog_offers_currency_code_fkey" FOREIGN KEY ("currency_code") REFERENCES "recommendations"."catalog_currencies"("currency_code");

--
-- Name: catalog_offers catalog_offers_group_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_offers"
    ADD CONSTRAINT "catalog_offers_group_id_fkey" FOREIGN KEY ("group_id") REFERENCES "recommendations"."catalog_groups"("group_id");

--
-- Name: catalog_offers catalog_offers_snapshot_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_offers"
    ADD CONSTRAINT "catalog_offers_snapshot_id_fkey" FOREIGN KEY ("snapshot_id") REFERENCES "recommendations"."catalog_snapshots"("id");

--
-- Name: catalog_offers catalog_offers_vendor_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."catalog_offers"
    ADD CONSTRAINT "catalog_offers_vendor_id_fkey" FOREIGN KEY ("vendor_id") REFERENCES "recommendations"."catalog_vendors"("vendor_id");

--
-- Name: historical_item_offers historical_item_offers_item_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."historical_item_offers"
    ADD CONSTRAINT "historical_item_offers_item_id_fkey" FOREIGN KEY ("item_id") REFERENCES "recommendations"."historical_items"("item_id");

--
-- Name: historical_item_offers historical_item_offers_offer_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."historical_item_offers"
    ADD CONSTRAINT "historical_item_offers_offer_id_fkey" FOREIGN KEY ("offer_id") REFERENCES "recommendations"."catalog_offers"("offer_id");

--
-- Name: licenses license_order_offer_fk; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."licenses"
    ADD CONSTRAINT "license_order_offer_fk" FOREIGN KEY ("order_item_id", "order_id", "offer_id") REFERENCES "recommendations"."store_order_items"("order_item_id", "order_id", "offer_id");

--
-- Name: licenses licenses_offer_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."licenses"
    ADD CONSTRAINT "licenses_offer_id_fkey" FOREIGN KEY ("offer_id") REFERENCES "recommendations"."catalog_offers"("offer_id");

--
-- Name: licenses licenses_order_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."licenses"
    ADD CONSTRAINT "licenses_order_id_fkey" FOREIGN KEY ("order_id") REFERENCES "recommendations"."orders"("order_id");

--
-- Name: licenses licenses_order_item_id_order_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."licenses"
    ADD CONSTRAINT "licenses_order_item_id_order_id_fkey" FOREIGN KEY ("order_item_id", "order_id") REFERENCES "recommendations"."store_order_items"("order_item_id", "order_id");

--
-- Name: order_deliveries order_deliveries_order_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_deliveries"
    ADD CONSTRAINT "order_deliveries_order_id_fkey" FOREIGN KEY ("order_id") REFERENCES "recommendations"."orders"("order_id");

--
-- Name: order_documents order_documents_order_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_documents"
    ADD CONSTRAINT "order_documents_order_id_fkey" FOREIGN KEY ("order_id") REFERENCES "recommendations"."orders"("order_id");

--
-- Name: order_invoices order_invoices_currency_code_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_invoices"
    ADD CONSTRAINT "order_invoices_currency_code_fkey" FOREIGN KEY ("currency_code") REFERENCES "recommendations"."catalog_currencies"("currency_code");

--
-- Name: order_invoices order_invoices_order_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_invoices"
    ADD CONSTRAINT "order_invoices_order_id_fkey" FOREIGN KEY ("order_id") REFERENCES "recommendations"."orders"("order_id");

--
-- Name: order_observed_weeks order_observed_weeks_order_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_observed_weeks"
    ADD CONSTRAINT "order_observed_weeks_order_id_fkey" FOREIGN KEY ("order_id") REFERENCES "recommendations"."orders"("order_id");

--
-- Name: order_payments order_payments_currency_code_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_payments"
    ADD CONSTRAINT "order_payments_currency_code_fkey" FOREIGN KEY ("currency_code") REFERENCES "recommendations"."catalog_currencies"("currency_code");

--
-- Name: order_payments order_payments_order_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."order_payments"
    ADD CONSTRAINT "order_payments_order_id_fkey" FOREIGN KEY ("order_id") REFERENCES "recommendations"."orders"("order_id");

--
-- Name: orders orders_client_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."orders"
    ADD CONSTRAINT "orders_client_id_fkey" FOREIGN KEY ("client_id") REFERENCES "recommendations"."clients"("client_id");

--
-- Name: recommendation_events recommendation_events_run_id_offer_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."recommendation_events"
    ADD CONSTRAINT "recommendation_events_run_id_offer_id_fkey" FOREIGN KEY ("run_id", "offer_id") REFERENCES "recommendations"."recommendation_items"("run_id", "offer_id");

--
-- Name: recommendation_items recommendation_items_offer_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."recommendation_items"
    ADD CONSTRAINT "recommendation_items_offer_id_fkey" FOREIGN KEY ("offer_id") REFERENCES "recommendations"."catalog_offers"("offer_id");

--
-- Name: recommendation_items recommendation_items_run_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."recommendation_items"
    ADD CONSTRAINT "recommendation_items_run_id_fkey" FOREIGN KEY ("run_id") REFERENCES "recommendations"."recommendation_runs"("run_id") ON DELETE CASCADE;

--
-- Name: recommendation_items recommendation_items_source_item_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."recommendation_items"
    ADD CONSTRAINT "recommendation_items_source_item_id_fkey" FOREIGN KEY ("source_item_id") REFERENCES "recommendations"."historical_items"("item_id");

--
-- Name: recommendation_runs recommendation_runs_client_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."recommendation_runs"
    ADD CONSTRAINT "recommendation_runs_client_id_fkey" FOREIGN KEY ("client_id") REFERENCES "recommendations"."clients"("client_id");

--
-- Name: sales sales_client_fk; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."sales"
    ADD CONSTRAINT "sales_client_fk" FOREIGN KEY ("client_id") REFERENCES "recommendations"."clients"("client_id");

--
-- Name: sales sales_item_fk; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."sales"
    ADD CONSTRAINT "sales_item_fk" FOREIGN KEY ("item_id") REFERENCES "recommendations"."historical_items"("item_id");

--
-- Name: sales sales_order_owner_fk; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."sales"
    ADD CONSTRAINT "sales_order_owner_fk" FOREIGN KEY ("order_id", "client_id") REFERENCES "recommendations"."orders"("order_id", "client_id");

--
-- Name: sales sales_order_week_fk; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."sales"
    ADD CONSTRAINT "sales_order_week_fk" FOREIGN KEY ("order_id", "order_week") REFERENCES "recommendations"."order_observed_weeks"("order_id", "order_week");

--
-- Name: store_order_items store_order_items_currency_code_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."store_order_items"
    ADD CONSTRAINT "store_order_items_currency_code_fkey" FOREIGN KEY ("currency_code") REFERENCES "recommendations"."catalog_currencies"("currency_code");

--
-- Name: store_order_items store_order_items_offer_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."store_order_items"
    ADD CONSTRAINT "store_order_items_offer_id_fkey" FOREIGN KEY ("offer_id") REFERENCES "recommendations"."catalog_offers"("offer_id");

--
-- Name: store_order_items store_order_items_order_id_fkey; Type: FK CONSTRAINT; Schema: recommendations; Owner: -
--

ALTER TABLE ONLY "recommendations"."store_order_items"
    ADD CONSTRAINT "store_order_items_order_id_fkey" FOREIGN KEY ("order_id") REFERENCES "recommendations"."orders"("order_id");

--
-- PostgreSQL database dump complete
--

END IF;
END
$bootstrap$;

SELECT pg_catalog.set_config('search_path', '"$user", public', false);
