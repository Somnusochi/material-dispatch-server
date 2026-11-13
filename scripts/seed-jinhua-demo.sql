-- Explicit local demo seed. One atomic statement; never run on application startup.
-- WGS84 points are synthetic positions inside the existing licensed Jinhua county data.
-- Boundary source: CTAmap v2.0 (Cheng et al.), CC BY 4.0; see the web project's public/data/README.md.
DO $seed$
DECLARE
  batch constant text := 'jinhua-counterweight-100t-v1';
  note constant text := '模拟数据：金华15地点100t配重块；非真实仓库，仅用于本地演示与测试。';
  spec bigint;
  rows jsonb := '[
    {"code":"DEMO-JH-100T-01","name":"模拟·婺城区配重仓库","loc_type":"warehouse","district":"婺城区","longitude":119.4707828,"latitude":28.9515774,"normal_qty":12},
    {"code":"DEMO-JH-100T-02","name":"模拟·婺城区作业堆场","loc_type":"yard","district":"婺城区","longitude":119.5307828,"latitude":28.9915774,"normal_qty":8},
    {"code":"DEMO-JH-100T-03","name":"模拟·金东区配重仓库","loc_type":"warehouse","district":"金东区","longitude":119.7958073,"latitude":29.1516455,"normal_qty":10},
    {"code":"DEMO-JH-100T-04","name":"模拟·金东区作业堆场","loc_type":"yard","district":"金东区","longitude":119.8558073,"latitude":29.1916455,"normal_qty":7},
    {"code":"DEMO-JH-100T-05","name":"模拟·兰溪市配重仓库","loc_type":"warehouse","district":"兰溪市","longitude":119.4213445,"latitude":29.2498804,"normal_qty":8},
    {"code":"DEMO-JH-100T-06","name":"模拟·兰溪市作业堆场","loc_type":"yard","district":"兰溪市","longitude":119.4813445,"latitude":29.2898804,"normal_qty":6},
    {"code":"DEMO-JH-100T-07","name":"模拟·义乌市配重仓库","loc_type":"warehouse","district":"义乌市","longitude":120.0130019,"latitude":29.2594610,"normal_qty":8},
    {"code":"DEMO-JH-100T-08","name":"模拟·义乌市作业堆场","loc_type":"yard","district":"义乌市","longitude":120.0730019,"latitude":29.2994610,"normal_qty":6},
    {"code":"DEMO-JH-100T-09","name":"模拟·东阳市配重仓库","loc_type":"warehouse","district":"东阳市","longitude":120.3125615,"latitude":29.2000461,"normal_qty":7},
    {"code":"DEMO-JH-100T-10","name":"模拟·东阳市作业堆场","loc_type":"yard","district":"东阳市","longitude":120.3725615,"latitude":29.2400461,"normal_qty":5},
    {"code":"DEMO-JH-100T-11","name":"模拟·永康市配重仓库","loc_type":"warehouse","district":"永康市","longitude":120.1013114,"latitude":28.9348716,"normal_qty":6},
    {"code":"DEMO-JH-100T-12","name":"模拟·永康市作业堆场","loc_type":"yard","district":"永康市","longitude":120.1613114,"latitude":28.9748716,"normal_qty":4},
    {"code":"DEMO-JH-100T-13","name":"模拟·浦江县配重仓库","loc_type":"warehouse","district":"浦江县","longitude":119.8835517,"latitude":29.5354235,"normal_qty":5},
    {"code":"DEMO-JH-100T-14","name":"模拟·武义县配重仓库","loc_type":"warehouse","district":"武义县","longitude":119.6306116,"latitude":28.6962978,"normal_qty":4},
    {"code":"DEMO-JH-100T-15","name":"模拟·磐安县配重仓库","loc_type":"warehouse","district":"磐安县","longitude":120.5624731,"latitude":28.9928249,"normal_qty":4}
  ]'::jsonb;
BEGIN
  -- CI PostgreSQL sees its container bridge address even when the client uses localhost.
  IF current_database() <> 'material_dispatch_test' AND NOT (
    current_database() = 'material_dispatch'
    AND (inet_server_addr() IS NULL OR inet_server_addr() IN ('127.0.0.1'::inet, '::1'::inet))
  ) THEN
    RAISE EXCEPTION 'Demo seed requires the dedicated local development/test database';
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(batch, 0));
  IF EXISTS (SELECT 1 FROM material_dispatch.operation_logs WHERE module='demo' AND action='seed' AND target_id=batch) THEN
    IF (SELECT count(*) FROM material_dispatch.locations WHERE code LIKE 'DEMO-JH-100T-%') <> 15
       OR (SELECT count(*) FROM material_dispatch.material_inventories i JOIN material_dispatch.material_specs s ON s.id=i.spec_id WHERE s.spec_code='DEMO-JH-100T-CW') <> 15 THEN
      RAISE EXCEPTION 'Existing demo data is incomplete; refusing to overwrite or reconstruct it';
    END IF;
    RAISE NOTICE 'Demo batch already exists; existing balances and changes are preserved';
    RETURN;
  END IF;
  IF EXISTS (SELECT 1 FROM material_dispatch.locations WHERE code LIKE 'DEMO-JH-100T-%')
     OR EXISTS (SELECT 1 FROM material_dispatch.material_specs WHERE spec_code='DEMO-JH-100T-CW') THEN
    RAISE EXCEPTION 'Demo code already in use; refusing to overwrite existing data';
  END IF;
  IF jsonb_array_length(rows) <> 15 OR (SELECT sum((r->>'normal_qty')::integer) FROM jsonb_array_elements(rows) r) <> 100 THEN
    RAISE EXCEPTION 'Invalid demo fixture: expected 15 locations and 100t';
  END IF;
  INSERT INTO material_dispatch.material_specs(spec_code,name,category,unit_weight_kg,unit,remark)
  VALUES('DEMO-JH-100T-CW','配重块（模拟）','counterweight',1000,'t',note)
  RETURNING id INTO spec;

  INSERT INTO material_dispatch.locations(code,name,loc_type,address,longitude,latitude,coordinate_system,status,remark)
  SELECT r.code,r.name,r.loc_type,'金华市'||r.district||'（模拟点位，非真实仓库）',r.longitude,r.latitude,'WGS84',1,note
  FROM jsonb_to_recordset(rows) AS r(code text,name text,loc_type text,district text,longitude numeric,latitude numeric);

  INSERT INTO material_dispatch.material_inventories(location_id,spec_id,normal_qty,damaged_qty,in_transit_qty)
  SELECT l.id,spec,r.normal_qty,0,0
  FROM jsonb_to_recordset(rows) AS r(code text,normal_qty integer)
  JOIN material_dispatch.locations l ON l.code=r.code;

  INSERT INTO material_dispatch.operation_logs(username,module,action,target_id,summary,request_id,request_method,request_path,after_data)
  VALUES('local-demo','demo','seed',batch,'创建金华15地点100t配重块模拟数据',batch,'CLI','scripts/seed-jinhua-demo.sql',
    jsonb_build_object('spec_id',spec::text,'spec_code','DEMO-JH-100T-CW','unit','t','normal_qty',100,'locations',
      (SELECT jsonb_agg(jsonb_build_object('id',l.id::text,'code',l.code,'name',l.name,'normal_qty',i.normal_qty) ORDER BY l.code)
       FROM material_dispatch.locations l JOIN material_dispatch.material_inventories i ON i.location_id=l.id WHERE i.spec_id=spec)));
END;
$seed$;
