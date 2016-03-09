package db

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
)

const (
	Redis    = "redis"
	Zk       = "zk"
	Zk2      = "zk2"
	ZkClient = "zk-client"

	RedisPortBase     = 6379
	ZkPortBase        = 2888
	ZkPortBase2       = 3888
	ZkPortBaseClient  = 2181
	RancherServerPort = 18080
)

var (
	log = logrus.WithField("component", "db")
)

type Member struct {
	ID             int
	Name           string
	UUID           string
	IP             string
	ports          string
	RequestedIndex int
	Heartbeat      int
	Index          int
}

func (m Member) PortByService(service string) int {
	zk, zk2, zkClient, redis := m.parsePorts()
	switch service {
	case Zk:
		return zk
	case Zk2:
		return zk2
	case ZkClient:
		return zkClient
	case Redis:
		return redis
	}
	return -1
}

func (m Member) parsePorts() (int, int, int, int) {
	ports := []int{}
	parts := strings.Split(m.ports, ",")
	for _, p := range parts {
		port, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		ports = append(ports, port)
	}

	if len(ports) == 4 {
		return ports[0], ports[1], ports[2], ports[3]
	}

	return ZkPortBase, ZkPortBase2, ZkPortBaseClient, RedisPortBase
}

type DB struct {
	db *sql.DB
}

func New(driverName, dsn string) (*DB, error) {
	db, err := sql.Open(driverName, dsn)
	return &DB{
		db: db,
	}, err
}

type Members []Member

func (a Members) Len() int           { return len(a) }
func (a Members) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Members) Less(i, j int) bool { return a[i].ID < a[j].ID }

func (d *DB) Migrate() error {
	_, err := d.Members()
	if err == nil {
		return nil
	}

	_, err = d.db.Exec("CREATE TABLE IF NOT EXISTS `cluster` (" +
		"`id` bigint(20) NOT NULL AUTO_INCREMENT," +
		"`name` varchar(256) DEFAULT NULL," +
		"`heartbeat` bigint(20) NOT NULL," +
		"`uuid` varchar(128) NOT NULL," +
		"`ip_address` varchar(128) NOT NULL," +
		"`requested_index` int(11) NOT NULL," +
		"`assigned_index` int(11) NOT NULL," +
		"`ports` varchar(128) NOT NULL," +
		" PRIMARY KEY (id)" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8;")
	return err
}

func (d *DB) Members() ([]Member, error) {
	rows, err := d.db.Query(`SELECT 
			id, name, heartbeat, uuid, assigned_index, requested_index, ports, ip_address
		FROM cluster ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Member{}

	for rows.Next() {
		member := Member{}
		if err := rows.Scan(&member.ID, &NullStringWrapper{String: &member.Name}, &member.Heartbeat, &member.UUID, &member.Index, &member.RequestedIndex, &member.ports,
			&member.IP); err != nil {
			return nil, err
		}
		result = append(result, member)
	}

	return result, rows.Err()
}

func (d *DB) APIKeys() (string, string, error) {
	for {
		a, b, err := d.apiKeys()
		if err != nil {
			return "", "", err
		}

		if a == "" && b == "" {
			log.Infof("Waiting for API keys for service account")
			return "", "", errors.New("Waiting for API keys for service account")
		}

		return a, b, err
	}
}
func (d *DB) apiKeys() (string, string, error) {
	rows, err := d.db.Query(`SELECT public_value, secret_value FROM credential c
		JOIN account a
		  ON (c.account_id = a.id)
		WHERE
		  c.state = ?
		  AND a.state = ?
		  AND a.uuid = ?`, "active", "active", "machineServiceAccount")
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	for rows.Next() {
		var accessKey, secretKey string
		if err := rows.Scan(&accessKey, &secretKey); err != nil {
			return "", "", err
		}
		return accessKey, secretKey, err
	}

	return "", "", nil
}

func (d *DB) Delete(uuid string) error {
	_, err := d.execCount(`DELETE FROM cluster WHERE uuid = ?`, uuid)
	return err
}

func (d *DB) Checkin(member Member, i int) error {
	count, err := d.execCount(`UPDATE cluster SET heartbeat = ? WHERE uuid = ?`, i, member.UUID)
	if err != nil {
		return err
	}

	if count == 0 {
		_, err := d.execCount(`INSERT INTO cluster(name,uuid,ip_address,requested_index) values(?, ?, ?, ?)`,
			member.Name, member.UUID, member.IP, member.RequestedIndex)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *DB) SaveIndex(indexes map[int]Member) error {
	for index, member := range indexes {
		_, err := d.execCount(`UPDATE cluster SET  assigned_index = ?, requested_index = ? WHERE ID = ?`,
			index, 0, member.ID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) execCount(sql string, args ...interface{}) (int64, error) {
	res, err := d.db.Exec(sql, args...)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

type NullStringWrapper struct {
	sql.NullString
	String *string
}

func (n *NullStringWrapper) Scan(value interface{}) error {
	if err := n.NullString.Scan(value); err != nil {
		return err
	}
	n.String = &n.NullString.String
	return nil
}
